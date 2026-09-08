package environments

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO workflow(id,name) VALUES(1,'Test')`, `INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Backlog','open','gray','circle',1)`, `INSERT INTO project(id,name,workflow) VALUES(1,'Test',1)`, `INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Test',1)`, `INSERT INTO task(id,checklist,stage,type,name,priority) SELECT 1,1,1,id,'Test','Medium' FROM task_type ORDER BY isDefault DESC,id LIMIT 1`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return &Store{DB: db}
}
func repoFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	return root
}
func configureFixture(t *testing.T, store *Store, root string) {
	t.Helper()
	if _, err := store.DB.Exec(`UPDATE project SET repoPath=? WHERE id=1`, root); err != nil {
		t.Fatal(err)
	}
	s := Defaults()
	s.Context = "kind-test"
	s.KindCluster = "test"
	s.Profile = "custom"
	raw, _ := json.Marshal(s)
	if _, err := store.DB.Exec(`INSERT INTO environment_settings(project,settings) VALUES(1,?)`, string(raw)); err != nil {
		t.Fatal(err)
	}
}
func TestEnvironmentIntentOwnershipAndSettingsSnapshots(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "hello"})
	configureFixture(t, store, root)
	s := &Service{Store: store}
	ctx := context.Background()
	first, err := s.Create(ctx, 1, CreateInput{Branch: "main", RequestKey: "same"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, 1, CreateInput{Branch: "main", RequestKey: "same"})
	if err != nil || first.ID != second.ID {
		t.Fatal("create is not idempotent")
	}
	if _, err = store.UpdateSettings(1, map[string]json.RawMessage{"memoryMiB": json.RawMessage(`1024`)}); err != nil {
		t.Fatal(err)
	}
	frozen, _ := store.Get(first.ID)
	if frozen.Settings.Memory != 768 {
		t.Fatal("existing recipe changed")
	}
	if _, err = s.Create(ctx, 1, CreateInput{Branch: "main", RequestKey: "second"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, 1, CreateInput{Branch: "main", RequestKey: "third"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("capacity ignored: %v", err)
	}
	if err = store.Action(first.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	first.State = "ready"
	first.URL = "http://127.0.0.1:1234"
	if err = store.Observe(first); err != nil {
		t.Fatal(err)
	}
	fresh, _ := store.Get(first.ID)
	if fresh.State != "stopping" || fresh.URL != "" {
		t.Fatal("stale observation undid stop")
	}
	result, err := store.Bulk(1, []int{first.ID, first.ID, 999}, "delete")
	if err != nil || result.Success != 1 || result.Skipped != 1 {
		t.Fatalf("bulk result: %#v %v", result, err)
	}
	for _, query := range []string{`DELETE FROM task WHERE checklist=1`, `DELETE FROM checklist WHERE project=1`, `DELETE FROM project WHERE id=1`} {
		if _, err = store.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.List(0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Desired != "deleted" || row.ProjectID != 0 {
			t.Fatal("project deletion orphaned runtime resources")
		}
	}
}
func TestEnvironmentSettingsPermitIncrementalEditsAndRejectUnsafeValues(t *testing.T) {
	store := testStore(t)
	if _, err := store.UpdateSettings(1, map[string]json.RawMessage{"autoPreview": json.RawMessage(`true`)}); !errors.Is(err, ErrInvalid) {
		t.Fatal("automatic work enabled without context")
	}
	if _, err := store.UpdateSettings(1, map[string]json.RawMessage{"testTarget": json.RawMessage(`""`)}); err != nil {
		t.Fatal(err)
	}
	s, err := store.UpdateSettings(1, map[string]json.RawMessage{"testCommand": json.RawMessage(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	if s.TestTarget != "" || len(s.TestCommand) != 0 {
		t.Fatal("incremental updates overwritten")
	}
	for key, value := range map[string]string{"port": "80", "cpuMillis": "0", "context": "\"--kubeconfig=evil\"", "dockerfile": "\"\"", "unknown": "true"} {
		if _, err := store.UpdateSettings(1, map[string]json.RawMessage{key: json.RawMessage(value)}); err == nil {
			t.Fatalf("accepted %s", key)
		}
	}
}

type fakeRuntime struct {
	mu         sync.Mutex
	started    chan struct{}
	blockBuild bool
	buildError error
	stops      int
	deletes    int
}

func (f *fakeRuntime) Build(ctx context.Context, e *Environment, dir string, log func(string)) (string, string, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.blockBuild {
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	return "image", "", f.buildError
}
func (f *fakeRuntime) Start(context.Context, *Environment, func(string)) error { return nil }
func (f *fakeRuntime) Inspect(context.Context, *Environment) (Observation, error) {
	return Observation{Ready: true, URL: "http://127.0.0.1:1234"}, nil
}
func (f *fakeRuntime) Stop(context.Context, *Environment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}
func (f *fakeRuntime) Destroy(context.Context, *Environment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	return nil
}
func (f *fakeRuntime) Logs(context.Context, *Environment) (string, error) { return "", nil }
func (f *fakeRuntime) Close()                                             {}

func TestStopDuringBuildCancelsAndConverges(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "hello"})
	configureFixture(t, store, root)
	runtime := &fakeRuntime{started: make(chan struct{}), blockBuild: true}
	s := &Service{Store: store, Runtime: runtime, Root: t.TempDir(), operations: map[int]operation{}, builds: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runtime.started:
	case <-time.After(5 * time.Second):
		t.Fatal("build never started")
	}
	if err = store.Action(e.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if err = s.tick(ctx); err != nil {
			t.Fatal(err)
		}
		current, _ := store.Get(e.ID)
		if current.State == "stopped" {
			s.wg.Wait()
			return
		}
	}
	t.Fatal("stop did not converge")
}

type readinessRuntime struct {
	fakeRuntime
	inspections int
	starts      int
}

func (f *readinessRuntime) Start(context.Context, *Environment, func(string)) error {
	f.starts++
	return nil
}
func (f *readinessRuntime) Inspect(context.Context, *Environment) (Observation, error) {
	f.inspections++
	switch f.inspections {
	case 1:
		return Observation{}, errors.New("Kubernetes API temporarily unavailable")
	case 2:
		return Observation{Message: "Waiting for readiness"}, nil
	default:
		return Observation{Ready: true, URL: "http://127.0.0.1:5678"}, nil
	}
}

func TestReadyPreviewRecoversWithoutRestartingWorkload(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "hello"})
	configureFixture(t, store, root)
	if _, err := store.UpdateSettings(1, map[string]json.RawMessage{"maxRunning": json.RawMessage(`1`)}); err != nil {
		t.Fatal(err)
	}
	runtime := &readinessRuntime{}
	s := &Service{Store: store, Runtime: runtime, Root: t.TempDir(), operations: map[int]operation{}}
	ctx := context.Background()
	e, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	e.State, e.Image, e.Digest, e.URL = "ready", "image", "captured", "http://127.0.0.1:1234"
	e.TestState = "passed"
	if err = store.Observe(e); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"unavailable", "unavailable", "ready"} {
		if err = s.tick(ctx); err != nil {
			t.Fatal(err)
		}
		s.wg.Wait()
		current, err := store.Get(e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State != state || current.TestState != "passed" {
			t.Fatalf("unexpected observation: %#v", current)
		}
		if state == "unavailable" && (current.URL != "" || current.Error == "") {
			t.Fatal("unavailable preview retained a broken URL or lost its error")
		}
		if state == "ready" && (current.URL != "http://127.0.0.1:5678" || current.Error != "") {
			t.Fatal("recovered preview did not restore its URL and clear the error")
		}
		if _, err = s.Create(ctx, 1, CreateInput{Branch: "main"}); !errors.Is(err, ErrConflict) {
			t.Fatalf("readiness loss released the running reservation: %v", err)
		}
	}
	if runtime.inspections != 3 || runtime.starts != 0 {
		t.Fatalf("recovery restarted work or stopped polling: inspections=%d starts=%d", runtime.inspections, runtime.starts)
	}
}

func TestCapacityRemainsReservedUntilCleanupCompletes(t *testing.T) {
	for _, action := range []string{"stop", "delete"} {
		t.Run(action, func(t *testing.T) {
			store := testStore(t)
			root := repoFixture(t, map[string]string{"app.txt": "hello"})
			configureFixture(t, store, root)
			if _, err := store.UpdateSettings(1, map[string]json.RawMessage{"maxRunning": json.RawMessage(`1`)}); err != nil {
				t.Fatal(err)
			}
			s := &Service{Store: store, Runtime: &fakeRuntime{}, Root: t.TempDir()}
			ctx := context.Background()
			e, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
			if err != nil {
				t.Fatal(err)
			}
			e.State = "failed"
			if err = store.Observe(e); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Create(ctx, 1, CreateInput{Branch: "main"}); !errors.Is(err, ErrConflict) {
				t.Fatalf("failed workload released capacity without cleanup: %v", err)
			}
			// Retrying the same failed environment consumes its existing slot.
			if err = store.Action(e.ID, "start"); err != nil {
				t.Fatalf("restart counted its own reservation twice: %v", err)
			}
			if err = store.Action(e.ID, action); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Create(ctx, 1, CreateInput{Branch: "main"}); !errors.Is(err, ErrConflict) {
				t.Fatalf("pending cleanup released capacity: %v", err)
			}
			e, err = store.Get(e.ID)
			if err != nil {
				t.Fatal(err)
			}
			s.reconcile(ctx, e)
			if _, err = s.Create(ctx, 1, CreateInput{Branch: "main"}); err != nil {
				t.Fatalf("completed cleanup did not release capacity: %v", err)
			}
		})
	}
}

func TestManifestsKeepWorkloadCredentialsAndStorageSeparate(t *testing.T) {
	e := &Environment{ID: 7, Settings: Defaults(), Image: "preview", TestImage: "tests"}
	app := deployment("instance", "http://127.0.0.1:8000", e)
	pod := app["spec"].(object)["template"].(object)["spec"].(object)
	if pod["automountServiceAccountToken"] != false {
		t.Fatal("preview gets Kubernetes credentials")
	}
	tests := testJob("instance", e)["spec"].(object)["template"].(object)["spec"].(object)
	raw, _ := json.Marshal(tests)
	if strings.Contains(string(raw), "persistentVolumeClaim") || strings.Contains(string(raw), "hostPath") || strings.Contains(string(raw), "privileged") {
		t.Fatal("test job inherited preview data or host mounts")
	}
	if namespace("a", e) == namespace("b", e) {
		t.Fatal("installations share namespaces")
	}
}
