// Package environments owns immutable source previews and their Kubernetes lifecycle.
// The runtime adapter handles external effects; Store records intent first so the
// controller can reconcile interrupted operations without duplicating environments.
package environments

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

var ErrInvalid = errors.New("invalid environment configuration")
var ErrConflict = errors.New("environment action unavailable")

//go:embed aycorn.Dockerfile
var AycornDockerfile string

type Settings struct {
	Context             string   `json:"context"`
	Transport           string   `json:"transport"`
	KindCluster         string   `json:"kindCluster"`
	ImageRepository     string   `json:"imageRepository"`
	PullSecretNamespace string   `json:"pullSecretNamespace"`
	PullSecretName      string   `json:"pullSecretName"`
	Platform            string   `json:"platform"`
	Profile             string   `json:"profile"`
	Dockerfile          string   `json:"dockerfile"`
	PreviewTarget       string   `json:"previewTarget"`
	TestTarget          string   `json:"testTarget"`
	TestCommand         []string `json:"testCommand"`
	Command             []string `json:"command"`
	Port                int      `json:"port"`
	HealthPath          string   `json:"healthPath"`
	CPU                 int      `json:"cpuMillis"`
	Memory              int      `json:"memoryMiB"`
	TestCPU             int      `json:"testCpuMillis"`
	TestMemory          int      `json:"testMemoryMiB"`
	StorageGiB          int      `json:"storageGiB"`
	StorageClass        string   `json:"storageClass"`
	TimeoutSeconds      int      `json:"timeoutSeconds"`
	MaxRunning          int      `json:"maxRunning"`
	RetentionHours      int      `json:"retentionHours"`
	AutoPreview         bool     `json:"autoPreview"`
	AllowTestNetwork    bool     `json:"allowTestNetwork"`
}

func Defaults() Settings {
	arch := runtime.GOARCH
	if arch != "arm64" {
		arch = "amd64"
	}
	return Settings{Transport: "kind", KindCluster: "aycorn", Platform: "linux/" + arch, Profile: "aycorn", Dockerfile: AycornDockerfile,
		PreviewTarget: "preview", TestTarget: "test", TestCommand: []string{"/bin/sh", "-c", "cd /src/app && npm test && cd /src/server && go test ./..."},
		Command: []string{}, Port: 8000, HealthPath: "/api/health/ready", CPU: 1000, Memory: 768, TestCPU: 2000, TestMemory: 4096,
		StorageGiB: 1, TimeoutSeconds: 1200, MaxRunning: 2, RetentionHours: 24}
}

var dnsName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
var imageRepo = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]*[a-z0-9]$`)
var targetName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (s Settings) Validate(ready bool) error {
	bad := func(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }
	if len(s.Context) > 200 || strings.HasPrefix(s.Context, "-") || strings.ContainsAny(s.Context, "\r\n\x00") {
		return bad("invalid Kubernetes context")
	}
	if ready && s.Context == "" {
		return bad("select a Kubernetes context in Project Settings → Environments")
	}
	if s.Transport != "kind" && s.Transport != "registry" {
		return bad("image transport must be kind or registry")
	}
	if s.Transport == "kind" && (!dnsName.MatchString(s.KindCluster) || len(s.KindCluster) > 50) {
		return bad("enter a valid kind cluster name")
	}
	if s.Transport == "kind" && ready && s.Context != "kind-"+s.KindCluster {
		return bad("the Kubernetes context must match the kind cluster (kind-<name>)")
	}
	if s.Transport == "registry" && (s.ImageRepository != "" || ready) && (!imageRepo.MatchString(s.ImageRepository) || strings.Contains(s.ImageRepository, "://") || strings.Contains(s.ImageRepository, "..") || len(s.ImageRepository) > 200) {
		return bad("enter a registry image repository without a tag, such as ghcr.io/you/aycorn-preview")
	}
	for _, v := range []string{s.PullSecretNamespace, s.PullSecretName, s.StorageClass} {
		if v != "" && (!dnsName.MatchString(v) || len(v) > 63) {
			return bad("invalid storage class or pull secret name")
		}
	}
	if ready && (s.PullSecretNamespace == "") != (s.PullSecretName == "") {
		return bad("provide both the pull secret namespace and name")
	}
	if s.Platform != "linux/amd64" && s.Platform != "linux/arm64" {
		return bad("choose linux/amd64 or linux/arm64")
	}
	if s.Profile != "aycorn" && s.Profile != "custom" {
		return bad("unknown environment profile")
	}
	if len(s.Dockerfile) == 0 || len(s.Dockerfile) > 65536 {
		return bad("a Dockerfile recipe of at most 64 KiB is required")
	}
	if !targetName.MatchString(s.PreviewTarget) || (s.TestTarget != "" && !targetName.MatchString(s.TestTarget)) {
		return bad("invalid Docker build target")
	}
	if ready && (s.TestTarget == "") != (len(s.TestCommand) == 0) {
		return bad("configure both a test target and command, or leave both empty")
	}
	for _, command := range [][]string{s.Command, s.TestCommand} {
		if len(command) > 64 {
			return bad("too many command arguments")
		}
		for _, arg := range command {
			if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
				return bad("invalid command argument")
			}
		}
	}
	if s.Port < 1024 || s.Port > 65535 || !strings.HasPrefix(s.HealthPath, "/") || strings.HasPrefix(s.HealthPath, "//") || len(s.HealthPath) > 256 || strings.ContainsAny(s.HealthPath, "\r\n?#") {
		return bad("use an unprivileged container port and a readiness URL path")
	}
	if s.CPU < 100 || s.CPU > 8000 || s.Memory < 128 || s.Memory > 16384 || s.TestCPU < 100 || s.TestCPU > 8000 || s.TestMemory < 256 || s.TestMemory > 16384 {
		return bad("resource limits are outside the supported range")
	}
	if s.StorageGiB < 1 || s.StorageGiB > 32 || s.TimeoutSeconds < 30 || s.TimeoutSeconds > 3600 || s.MaxRunning < 1 || s.MaxRunning > 10 || s.RetentionHours < 1 || s.RetentionHours > 720 {
		return bad("storage, timeout, capacity, or retention is outside the supported range")
	}
	return nil
}

type Environment struct {
	ID             int      `json:"id"`
	ProjectID      int      `json:"projectId"`
	TaskID         int      `json:"taskId"`
	JobID          int      `json:"jobId"`
	RequestKey     string   `json:"-"`
	Name           string   `json:"name"`
	Repo           string   `json:"-"`
	Branch         string   `json:"branch"`
	Commit         string   `json:"commit"`
	IncludeChanges bool     `json:"includeChanges"`
	Digest         string   `json:"digest"`
	Settings       Settings `json:"settings"`
	State          string   `json:"state"`
	Desired        string   `json:"desired"`
	Image          string   `json:"image"`
	TestImage      string   `json:"testImage"`
	TestState      string   `json:"testState"`
	TestExitCode   *int     `json:"testExitCode"`
	URL            string   `json:"url"`
	Error          string   `json:"error"`
	Pinned         bool     `json:"pinned"`
	ExpiresAt      int64    `json:"expiresAt"`
	CreatedAt      int64    `json:"createdAt"`
	UpdatedAt      int64    `json:"updatedAt"`
}

type CreateInput struct {
	Branch         string `json:"branch"`
	TaskID         int    `json:"taskId"`
	JobID          int    `json:"jobId"`
	RequestKey     string `json:"requestKey"`
	IncludeChanges bool   `json:"includeChanges"`
}

type Observation struct {
	Ready   bool
	URL     string
	Message string
}
type Runtime interface {
	Build(context.Context, *Environment, string, func(string)) (string, string, error)
	Start(context.Context, *Environment, func(string)) error
	Inspect(context.Context, *Environment) (Observation, error)
	Stop(context.Context, *Environment) error
	Destroy(context.Context, *Environment) error
	Logs(context.Context, *Environment) (string, error)
	Close()
}
