package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worker"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func aiTestApp(t *testing.T, root string) *app {
	app, _ := requestAgentTestApp(t, root)
	s := app.taskService
	app.aiService = &services.AIService{Jobs: app.agentJobService.JobRepo, Tasks: s.TaskRepo, Presets: s.PersonaRepo, Projects: s.ProjectRepo, Converter: &markdown.Converter{}, Probe: func(context.Context, string) harness.EngineHealth {
		return harness.EngineHealth{Ready: true, Executable: "/test/engine", Version: "test"}
	}}
	return app
}
func TestAIRunRequestIsExplicitAndSnapshotsContext(t *testing.T) {
	app := aiTestApp(t, "")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/ai/tasks/1/runs", strings.NewReader(`{"intent":"ask","instruction":"Explain this","presetId":1}`)))
	if rec.Code != 201 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var job models.AgentJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Request == nil || job.Request.Instruction != "Explain this" || job.Request.RepoPath != "" {
		t.Fatalf("bad snapshot: %+v", job)
	}
	// Preset deletion neither cancels nor deletes its run.
	if _, err := app.aiService.Jobs.DB.Exec("DELETE FROM persona WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	stored, err := app.aiService.Jobs.FindOne(job.ID)
	if err != nil || stored.Persona != 0 || stored.Request.PresetName != "p" {
		t.Fatalf("lost history: %+v %v", stored, err)
	}
	task, err := app.aiService.Tasks.FindOneWithProject(1)
	if err != nil || task.Assignee != "p" || task.Stage != 2 {
		t.Fatalf("changed task ownership: %+v %v", task, err)
	}
	rec = httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/ai/tasks/1/runs", strings.NewReader(`{"intent":"ask"}`)))
	if rec.Code != 409 {
		t.Fatalf("duplicate status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/ai/runs/"+fmtInt(job.ID)+"/cancel", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	stored, _ = app.aiService.Jobs.FindOne(job.ID)
	if stored.Status != "canceled" {
		t.Fatal(stored.Status)
	}
}
func fmtInt(v int) string { raw, _ := json.Marshal(v); return string(raw) }
func TestAIRunConcurrentRequestsAreAtomic(t *testing.T) {
	app := aiTestApp(t, "")
	var wg sync.WaitGroup
	statuses := make([]int, 6)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRecorder()
			app.routes().ServeHTTP(r, httptest.NewRequest("POST", "/api/ai/tasks/1/runs", strings.NewReader(`{"intent":"ask"}`)))
			statuses[i] = r.Code
		}(i)
	}
	wg.Wait()
	success := 0
	for _, status := range statuses {
		if status == 201 {
			success++
		} else if status != 409 {
			t.Fatalf("statuses %v", statuses)
		}
	}
	if success != 1 {
		t.Fatalf("statuses %v", statuses)
	}
}

type failingEditHarness struct{}

func (failingEditHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	if err := os.WriteFile(filepath.Join(spec.WorkDir, "new.txt"), []byte("preserve this partial change\n"), 0600); err != nil {
		return harness.RunResult{}, err
	}
	return harness.RunResult{Output: "Partial answer", UsageJson: "{}", ExitCode: 1}, errors.New("provider disconnected")
}
func TestAIFailurePreservesWorkspaceAndNewFilePatch(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if raw, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v %s", err, raw)
		}
	}
	app := aiTestApp(t, root)
	job, err := app.aiService.Start(context.Background(), 1, services.AIRunInput{Intent: "implement"})
	if err != nil {
		t.Fatal(err)
	}
	w := worker.New(app.agentJobService, app.taskService, failingEditHarness{})
	w.ExplicitOnly = true
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := app.agentJobService.ListByTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Runs) != 1 || data.Jobs[0].Status != "failed" || data.Runs[0].Output != "Partial answer" {
		t.Fatalf("%+v", data)
	}
	a := data.Runs[0].Artifacts
	if !strings.Contains(a.Diff, "+preserve this partial change") || a.Branch != "aycorn/run-"+job.Request.Key {
		t.Fatalf("missing artifacts: %+v", a)
	}
	if _, err := os.Stat(filepath.Join(a.Workspace, "new.txt")); err != nil {
		t.Fatal("lost workspace", err)
	}
}

type waitingHarness struct{ started chan struct{} }

func (h waitingHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	close(h.started)
	<-ctx.Done()
	return harness.RunResult{Output: "Kept on stop"}, ctx.Err()
}
func TestAIRunningCancelRetainsAttempt(t *testing.T) {
	app := aiTestApp(t, "")
	job, err := app.aiService.Start(context.Background(), 1, services.AIRunInput{Intent: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	h := waitingHarness{make(chan struct{})}
	w := worker.New(app.agentJobService, app.taskService, h)
	w.ExplicitOnly = true
	done := make(chan error, 1)
	go func() { _, err := w.RunOnce(context.Background()); done <- err }()
	select {
	case <-h.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	if _, err := app.aiService.Jobs.CancelAI(job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop run")
	}
	data, err := app.agentJobService.ListByTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if data.Jobs[0].Status != "canceled" || data.Runs[0].Output != "Kept on stop" {
		t.Fatalf("%+v", data)
	}
}
