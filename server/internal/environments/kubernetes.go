package environments

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Kubernetes struct {
	Token    string
	MainURL  string
	mu       sync.Mutex
	forwards map[int]*forward
}
type forward struct {
	cancel context.CancelFunc
	done   chan struct{}
	url    string
}

func (k *Kubernetes) kubectl(ctx context.Context, e *Environment, input []byte, args ...string) ([]byte, error) {
	if e.Settings.Context == "" {
		return nil, fmt.Errorf("Kubernetes context is required")
	}
	prefix := []string{"--context", e.Settings.Context, "--request-timeout=15s"}
	return command(ctx, input, nil, "kubectl", append(prefix, args...)...)
}
func (k *Kubernetes) apply(ctx context.Context, e *Environment, objects ...object) error {
	raw, err := json.Marshal(object{"apiVersion": "v1", "kind": "List", "items": objects})
	if err != nil {
		return err
	}
	_, err = k.kubectl(ctx, e, raw, "apply", "-f", "-")
	return err
}
func (k *Kubernetes) owned(ctx context.Context, e *Environment) (bool, error) {
	raw, err := k.kubectl(ctx, e, nil, "get", "namespace", namespace(k.Token, e), "--ignore-not-found", "-o", "json")
	if err != nil {
		return false, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return false, nil
	}
	var ns struct {
		Metadata struct{ Labels map[string]string }
	}
	if err = json.Unmarshal(raw, &ns); err != nil {
		return false, err
	}
	if ns.Metadata.Labels["aycorn.dev/installation"] != k.Token || ns.Metadata.Labels["aycorn.dev/environment"] != strconv.Itoa(e.ID) {
		return false, fmt.Errorf("refusing to modify a namespace owned by another installation")
	}
	return true, nil
}

func (k *Kubernetes) imageTag(e *Environment) string {
	repository := "aycorn-preview"
	if e.Settings.Transport == "registry" {
		repository = e.Settings.ImageRepository
	}
	return fmt.Sprintf("%s:%s-e%d", repository, k.Token, e.ID)
}

func (k *Kubernetes) Build(ctx context.Context, e *Environment, directory string, log func(string)) (string, string, error) {
	if err := e.Settings.Validate(true); err != nil {
		return "", "", err
	}
	log("Checking Kubernetes context " + e.Settings.Context + "\n")
	if _, err := k.kubectl(ctx, e, nil, "get", "--raw", "/readyz"); err != nil {
		return "", "", fmt.Errorf("cluster is unavailable: %w", err)
	}
	if e.Settings.Transport == "kind" {
		raw, err := command(ctx, nil, nil, "kind", "get", "clusters")
		if err != nil {
			return "", "", err
		}
		found := false
		for _, name := range strings.Fields(string(raw)) {
			if name == e.Settings.KindCluster {
				found = true
			}
		}
		if !found {
			return "", "", fmt.Errorf("kind cluster %q does not exist; create it using the Kubernetes setup guide", e.Settings.KindCluster)
		}
	}
	recipe := filepath.Join(directory, "Dockerfile")
	if err := os.WriteFile(recipe, []byte(e.Settings.Dockerfile), 0600); err != nil {
		return "", "", err
	}
	image := k.imageTag(e)
	testImage := ""
	build := func(target, tag string) error {
		log("\nBuilding " + target + " from the captured source\n")
		_, err := command(ctx, nil, log, "docker", "build", "--platform", e.Settings.Platform, "--progress=plain", "--label", "aycorn.dev/installation="+k.Token, "--label", fmt.Sprintf("aycorn.dev/environment=%d", e.ID), "--file", recipe, "--target", target, "--tag", tag, filepath.Join(directory, "source"))
		if err != nil {
			return err
		}
		if e.Settings.Transport == "kind" {
			_, err = command(ctx, nil, log, "kind", "load", "docker-image", "--name", e.Settings.KindCluster, tag)
		} else {
			_, err = command(ctx, nil, log, "docker", "push", tag)
		}
		return err
	}
	if err := build(e.Settings.PreviewTarget, image); err != nil {
		return "", "", err
	}
	if e.Settings.TestTarget != "" {
		testImage = image + "-test"
		if err := build(e.Settings.TestTarget, testImage); err != nil {
			return "", "", err
		}
	}
	return image, testImage, nil
}

func (k *Kubernetes) Start(ctx context.Context, e *Environment, log func(string)) error {
	if _, err := k.owned(ctx, e); err != nil {
		return err
	}
	if err := k.apply(ctx, e, foundation(k.Token, e)...); err != nil {
		return err
	}
	if e.Settings.PullSecretName != "" {
		if err := k.copyPullSecret(ctx, e); err != nil {
			return err
		}
	}
	if e.Settings.AllowTestNetwork {
		if err := k.apply(ctx, e, testNetwork(k.Token, e)); err != nil {
			return err
		}
	}
	if e.TestImage != "" {
		e.TestState = "running"
		if err := k.ensureTestJob(ctx, e); err != nil {
			return err
		}
		log("\nRunning the configured tests in a Kubernetes Job\n")
		for {
			state, exit, message, err := k.testStatus(ctx, e)
			if err != nil {
				return err
			}
			if state == "passed" || state == "failed" {
				e.TestState = state
				e.TestExitCode = exit
				output, _ := k.kubectl(ctx, e, nil, "-n", namespace(k.Token, e), "logs", "job/tests", "--tail=500", "--limit-bytes=131072")
				log(string(output))
				if state == "failed" {
					return fmt.Errorf("container tests failed: %s", message)
				}
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	} else {
		e.TestState = "skipped"
	}
	if err := k.apply(ctx, e, deployment(k.Token, k.MainURL, e), serviceManifest(k.Token, e)); err != nil {
		return err
	}
	log("\nWaiting for the application readiness check\n")
	return nil
}

func (k *Kubernetes) copyPullSecret(ctx context.Context, e *Environment) error {
	raw, err := k.kubectl(ctx, e, nil, "-n", e.Settings.PullSecretNamespace, "get", "secret", e.Settings.PullSecretName, "-o", "json")
	if err != nil {
		return fmt.Errorf("cannot read the configured registry pull secret")
	}
	var secret struct {
		Type string            `json:"type"`
		Data map[string]string `json:"data"`
	}
	if json.Unmarshal(raw, &secret) != nil || secret.Type != "kubernetes.io/dockerconfigjson" || secret.Data[".dockerconfigjson"] == "" {
		return fmt.Errorf("registry secret must have type kubernetes.io/dockerconfigjson")
	}
	manifest := resource("v1", "Secret", "registry", namespace(k.Token, e), labels(k.Token, e), nil)
	manifest["type"] = secret.Type
	manifest["data"] = object{".dockerconfigjson": secret.Data[".dockerconfigjson"]}
	if err = k.apply(ctx, e, manifest); err != nil {
		return fmt.Errorf("could not copy the registry pull secret into this environment")
	}
	return nil
}

func (k *Kubernetes) ensureTestJob(ctx context.Context, e *Environment) error {
	return k.apply(ctx, e, testJob(k.Token, e))
}
func (k *Kubernetes) testStatus(ctx context.Context, e *Environment) (string, *int, string, error) {
	raw, err := k.kubectl(ctx, e, nil, "-n", namespace(k.Token, e), "get", "job", "tests", "--ignore-not-found", "-o", "json")
	if err != nil {
		return "", nil, "", err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return "pending", nil, "", nil
	}
	var job struct {
		Status struct {
			Succeeded  int
			Failed     int
			Conditions []struct {
				Type    string
				Status  string
				Message string
				Reason  string
			}
		}
	}
	if err = json.Unmarshal(raw, &job); err != nil {
		return "", nil, "", err
	}
	if job.Status.Succeeded > 0 {
		zero := 0
		return "passed", &zero, "", nil
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == "Failed" && condition.Status == "True" {
			var exit *int
			pods, _ := k.kubectl(ctx, e, nil, "-n", namespace(k.Token, e), "get", "pods", "-l", "job-name=tests", "-o", "json")
			var result struct {
				Items []struct {
					Status struct {
						ContainerStatuses []struct {
							State struct{ Terminated *struct{ ExitCode int } }
						}
					}
				}
			}
			if json.Unmarshal(pods, &result) == nil {
				for _, pod := range result.Items {
					for _, container := range pod.Status.ContainerStatuses {
						if container.State.Terminated != nil {
							code := container.State.Terminated.ExitCode
							exit = &code
						}
					}
				}
			}
			return "failed", exit, condition.Reason + ": " + condition.Message, nil
		}
	}
	return "running", nil, "", nil
}

func (k *Kubernetes) Inspect(ctx context.Context, e *Environment) (Observation, error) {
	owned, err := k.owned(ctx, e)
	if err != nil {
		return Observation{}, err
	}
	if !owned {
		return Observation{Message: "Environment namespace is missing; restart to recreate it."}, nil
	}
	raw, err := k.kubectl(ctx, e, nil, "-n", namespace(k.Token, e), "get", "deployment", "app", "--ignore-not-found", "-o", "json")
	if err != nil {
		return Observation{}, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return Observation{Message: "Waiting for application deployment"}, nil
	}
	var deploy struct {
		Metadata struct{ Generation int64 }
		Status   struct {
			ObservedGeneration int64
			ReadyReplicas      int
			AvailableReplicas  int
			Conditions         []struct {
				Type    string
				Status  string
				Message string
			}
		}
	}
	if err = json.Unmarshal(raw, &deploy); err != nil {
		return Observation{}, err
	}
	if deploy.Status.ObservedGeneration < deploy.Metadata.Generation || deploy.Status.ReadyReplicas != 1 || deploy.Status.AvailableReplicas != 1 {
		message := "Waiting for the pod, storage, and application readiness"
		for _, condition := range deploy.Status.Conditions {
			if condition.Status == "False" && condition.Message != "" {
				message = condition.Message
			}
		}
		return Observation{Message: message}, nil
	}
	url, err := k.forward(ctx, e)
	if err != nil {
		return Observation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url+e.Settings.HealthPath, nil)
	if err != nil {
		return Observation{}, err
	}
	client := http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return Observation{Message: "Waiting for the local preview connection: " + err.Error()}, nil
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return Observation{Message: fmt.Sprintf("Readiness returned HTTP %d", response.StatusCode)}, nil
	}
	return Observation{Ready: true, URL: url}, nil
}

var forwardedPort = regexp.MustCompile(`Forwarding from 127\.0\.0\.1:([0-9]+) ->`)

func (k *Kubernetes) forward(ctx context.Context, e *Environment) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.forwards == nil {
		k.forwards = map[int]*forward{}
	}
	if current := k.forwards[e.ID]; current != nil {
		select {
		case <-current.done:
			delete(k.forwards, e.ID)
		default:
			return current.url, nil
		}
	}
	// This process intentionally outlives the inspection request. Close, Stop and
	// Destroy cancel it, and the next inspection repairs a dropped connection.
	lifetime, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(lifetime, "kubectl", "--context", e.Settings.Context, "-n", namespace(k.Token, e), "port-forward", "--address=127.0.0.1", "service/app", fmt.Sprintf(":%d", e.Settings.Port))
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", err
	}
	stderr := &tailBuffer{limit: 8192}
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		cancel()
		return "", err
	}
	f := &forward{cancel: cancel, done: make(chan struct{})}
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := forwardedPort.FindStringSubmatch(scanner.Text()); len(match) == 2 {
				select {
				case ready <- "http://127.0.0.1:" + match[1]:
				default:
				}
			}
		}
		_ = cmd.Wait()
		close(f.done)
	}()
	select {
	case url := <-ready:
		f.url = url
		k.forwards[e.ID] = f
		return url, nil
	case <-f.done:
		cancel()
		return "", fmt.Errorf("preview port-forward exited: %s", stderr.String())
	case <-ctx.Done():
		cancel()
		return "", ctx.Err()
	case <-time.After(10 * time.Second):
		cancel()
		return "", fmt.Errorf("timed out opening a localhost preview connection: %s", stderr.String())
	}
}
func (k *Kubernetes) closeForward(id int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if f := k.forwards[id]; f != nil {
		f.cancel()
		delete(k.forwards, id)
	}
}
func (k *Kubernetes) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()
	for id, f := range k.forwards {
		f.cancel()
		delete(k.forwards, id)
	}
}

func (k *Kubernetes) Stop(ctx context.Context, e *Environment) error {
	k.closeForward(e.ID)
	owned, err := k.owned(ctx, e)
	if err != nil || !owned {
		return err
	}
	ns := namespace(k.Token, e)
	raw, err := k.kubectl(ctx, e, nil, "-n", ns, "get", "deployment", "app", "--ignore-not-found", "-o", "name")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) != "" {
		if _, err = k.kubectl(ctx, e, nil, "-n", ns, "scale", "deployment/app", "--replicas=0"); err != nil {
			return err
		}
	}
	if _, err = k.kubectl(ctx, e, nil, "-n", ns, "delete", "job", "tests", "--ignore-not-found", "--wait=true", "--timeout=45s"); err != nil {
		return err
	}
	raw, err = k.kubectl(ctx, e, nil, "-n", ns, "get", "pods", "-l", "aycorn.dev/environment="+strconv.Itoa(e.ID), "-o", "name")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	_, err = k.kubectl(ctx, e, nil, "-n", ns, "wait", "--for=delete", "pod", "-l", "aycorn.dev/environment="+strconv.Itoa(e.ID), "--timeout=60s")
	return err
}
func (k *Kubernetes) Destroy(ctx context.Context, e *Environment) error {
	k.closeForward(e.ID)
	owned, err := k.owned(ctx, e)
	if err != nil {
		return err
	}
	if owned {
		if _, err = k.kubectl(ctx, e, nil, "delete", "namespace", namespace(k.Token, e), "--wait=true", "--timeout=60s"); err != nil {
			return err
		}
	}
	// Derive the tags even if a build failed before persisting its image IDs.
	// Remove only exact, locally owned tags. Registry retention belongs to the
	// registry; never invoke a global prune or delete unrelated images.
	image := k.imageTag(e)
	for _, tag := range []string{image, image + "-test"} {
		if err = k.removeImage(ctx, e, tag); err != nil {
			return err
		}
	}
	return nil
}

func (k *Kubernetes) removeImage(ctx context.Context, e *Environment, tag string) error {
	raw, err := command(ctx, nil, nil, "docker", "image", "inspect", "--format", `{{json .Config.Labels}}`, tag)
	if err != nil {
		// Docker inspect does not distinguish an absent image from a daemon or
		// permission failure by exit code. Confirm absence through a successful
		// exact-reference listing; other failures must remain retryable.
		listed, listErr := command(ctx, nil, nil, "docker", "image", "ls", "--quiet", "--filter", "reference="+tag)
		if listErr == nil && strings.TrimSpace(string(listed)) == "" {
			return nil
		}
		return fmt.Errorf("cannot inspect preview image %s for cleanup: %w", tag, err)
	}
	var imageLabels map[string]string
	if err = json.Unmarshal(raw, &imageLabels); err != nil {
		return fmt.Errorf("cannot read ownership of preview image %s: %w", tag, err)
	}
	if imageLabels["aycorn.dev/installation"] != k.Token || imageLabels["aycorn.dev/environment"] != strconv.Itoa(e.ID) {
		return fmt.Errorf("refusing to remove image %s owned by another installation or environment", tag)
	}
	if _, err = command(ctx, nil, nil, "docker", "image", "rm", tag); err != nil {
		return fmt.Errorf("cannot remove preview image %s: %w", tag, err)
	}
	return nil
}
func (k *Kubernetes) Logs(ctx context.Context, e *Environment) (string, error) {
	owned, err := k.owned(ctx, e)
	if err != nil || !owned {
		return "", err
	}
	ns := namespace(k.Token, e)
	var result strings.Builder
	for _, target := range []string{"job/tests", "deployment/app"} {
		raw, err := k.kubectl(ctx, e, nil, "-n", ns, "logs", target, "--all-containers=true", "--tail=150", "--limit-bytes=65536", "--pod-running-timeout=2s")
		if err == nil {
			fmt.Fprintf(&result, "\n%s\n%s", target, raw)
		}
	}
	events, err := k.kubectl(ctx, e, nil, "-n", ns, "get", "events", "--field-selector=type=Warning", "--sort-by=.lastTimestamp", "-o", "json")
	if err == nil {
		var list struct {
			Items []struct {
				Reason  string
				Message string
			}
		}
		if json.Unmarshal(events, &list) == nil {
			for _, event := range list.Items {
				fmt.Fprintf(&result, "\n%s: %s\n", event.Reason, event.Message)
			}
		}
	}
	return result.String(), nil
}

type ConnectionStatus struct {
	Kubectl        bool     `json:"kubectl"`
	Docker         bool     `json:"docker"`
	Kind           bool     `json:"kind"`
	Contexts       []string `json:"contexts"`
	StorageClasses []string `json:"storageClasses"`
	Connected      bool     `json:"connected"`
	Error          string   `json:"error"`
}

func (k *Kubernetes) Check(ctx context.Context, settings Settings) ConnectionStatus {
	status := ConnectionStatus{Contexts: []string{}, StorageClasses: []string{}}
	_, err := exec.LookPath("kubectl")
	status.Kubectl = err == nil
	_, err = exec.LookPath("kind")
	status.Kind = err == nil
	_, err = command(ctx, nil, nil, "docker", "info", "--format", "{{.ServerVersion}}")
	status.Docker = err == nil
	if !status.Kubectl {
		status.Error = "Install kubectl on the Aycorn host and put it on PATH."
		return status
	}
	raw, err := command(ctx, nil, nil, "kubectl", "config", "get-contexts", "-o", "name")
	if err == nil {
		status.Contexts = strings.Fields(string(raw))
	}
	if settings.Context == "" {
		status.Error = "Choose a Kubernetes context."
		return status
	}
	e := &Environment{Settings: settings}
	if _, err = k.kubectl(ctx, e, nil, "get", "--raw", "/readyz"); err != nil {
		status.Error = err.Error()
		return status
	}
	raw, err = k.kubectl(ctx, e, nil, "auth", "can-i", "create", "namespaces")
	if err != nil || strings.TrimSpace(string(raw)) != "yes" {
		status.Error = "This context needs permission to create preview namespaces; see the Kubernetes setup guide."
		return status
	}
	raw, err = k.kubectl(ctx, e, nil, "get", "storageclasses", "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	if err == nil {
		status.StorageClasses = strings.Fields(string(raw))
	}
	if !status.Docker {
		status.Error = "Docker is unavailable on the Aycorn host."
		return status
	}
	if settings.Transport == "kind" && !status.Kind {
		status.Error = "Install kind to load images into the local cluster."
		return status
	}
	status.Connected = true
	return status
}
