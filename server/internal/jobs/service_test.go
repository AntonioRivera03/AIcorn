package jobs

import (
	"context"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worker"
	_ "modernc.org/sqlite"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) *Service {
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
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Test')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1),(2,1,'Planning','todo','gray','circle',2),(3,1,'Doing','doing','gray','circle',3),(4,1,'Review','todo','gray','circle',4)`,
		`INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Test',0),(2,1,'Other',0)`,
		`INSERT INTO checklist(id,project,name) VALUES(1,1,'Test'),(2,2,'Other')`,
		`INSERT INTO project_task_type(project,task_type) VALUES(1,1),(2,1)`,
		`INSERT INTO persona(id,name,harness,model,system_prompt) VALUES(10,'Conductor','codex','gpt-6-astra','[]'),(11,'Job coder','codex','gpt-5.6-sol','[]')`,
		`UPDATE ai_settings SET model='gpt-5.6-sol'`,
		`UPDATE persona SET model='gpt-6-astra' WHERE builtin_role='conductor'`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ai := &services.AIService{Jobs: &repos.AgentJobRepo{DB: db}, Tasks: &repos.TaskRepo{DB: db}, Projects: &repos.ProjectRepo{DB: db}, Presets: &repos.PersonaRepo{DB: db}, Converter: &markdown.Converter{}, Probe: func(context.Context, string) harness.EngineHealth {
		return harness.EngineHealth{Ready: true, Executable: "fake"}
	}}
	c := &services.ConductorService{Repo: &repos.ConductorRepo{DB: db}, AI: ai, Runs: &repos.AgentRunRepo{DB: db}}
	settings := models.DefaultConductorSettings()
	settings.UseRepository = false
	settings.PlanningStage = 2
	settings.WorkingStage = 3
	settings.CompletionStage = 4
	settings.ConductorAgentID = 10
	if err = c.Repo.SaveSettings(1, settings, ""); err != nil {
		t.Fatal(err)
	}
	return &Service{DB: db, AI: ai, Conductor: c, Now: func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) }}
}
func makeJob(t *testing.T, s *Service, scheduled bool) (Template, Job) {
	t.Helper()
	tpl, err := s.CreateTemplate(1)
	if err != nil {
		t.Fatal(err)
	}
	tpl.Title = "Repo audit"
	tpl.Body = "Inspect dependencies"
	tpl.Prompt = "Report concrete findings"
	tpl, err = s.UpdateTemplate(1, tpl.ID, tpl)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.CreateJob(1, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	j.AgentID = 11
	j.Enabled = scheduled
	if scheduled {
		j.Schedule = "* * * * *"
	}
	j, err = s.UpdateJob(context.Background(), 1, j.ID, j)
	if err != nil {
		t.Fatal(err)
	}
	return tpl, j
}
func TestTemplateSnapshotsAndScope(t *testing.T) {
	s := fixture(t)
	tpl, j := makeJob(t, s, false)
	if tpl.ChecklistID != 1 || tpl.StageID != 1 || tpl.TypeID != 1 {
		t.Fatalf("missing defaults: %+v", tpl)
	}
	id, err := s.Instantiate(context.Background(), 1, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.AI.Tasks.FindOneWithProject(id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != tpl.Title || task.Stage != 1 || !strings.Contains(task.Body, tpl.Body) {
		t.Fatalf("bad task: %+v", task)
	}
	stale := tpl
	tpl.Body = "New body"
	tpl, err = s.UpdateTemplate(1, tpl.ID, tpl)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateTemplate(1, tpl.ID, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	task, _ = s.AI.Tasks.FindOneWithProject(id)
	if strings.Contains(task.Body, "New body") {
		t.Fatal("template edits mutated prior task")
	}
	tpl.ChecklistID = 2
	if _, err = s.UpdateTemplate(1, tpl.ID, tpl); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-project reference accepted: %v", err)
	}
	if _, err = s.Instantiate(context.Background(), 2, tpl.ID); err == nil {
		t.Fatal("cross-project access accepted")
	}
	if err = s.DeleteTemplate(1, tpl.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("in-use deletion: %v", err)
	}
	if err = s.DeleteJob(1, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteTemplate(1, tpl.ID); err != nil {
		t.Fatal(err)
	}
}
func TestRunNowConcurrentIdempotencyAndOverlap(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, false)
	var arrived sync.WaitGroup
	arrived.Add(2)
	s.AI.Probe = func(context.Context, string) harness.EngineHealth {
		arrived.Done()
		arrived.Wait()
		return harness.EngineHealth{Ready: true, Executable: "fake"}
	}
	ids := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.Fire(context.Background(), 1, j.ID, "manual", "same-click")
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first int
	for id := range ids {
		if id == 0 || (first != 0 && id != first) {
			t.Fatalf("different tasks: %d %d", first, id)
		}
		first = id
	}
	s.AI.Probe = func(context.Context, string) harness.EngineHealth {
		return harness.EngineHealth{Ready: true, Executable: "fake"}
	}
	if _, err := s.Fire(context.Background(), 1, j.ID, "manual", "new-click"); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap: %v", err)
	}
	if err := s.DeleteJob(1, j.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("deleted active job: %v", err)
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM task").Scan(&count)
	if count != 1 {
		t.Fatalf("tasks=%d", count)
	}
	if _, err := s.AI.Projects.DeleteProject(1); err != nil {
		t.Fatal(err)
	}
}
func TestSchedulerMissedCadenceOverlapAndFailure(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, true)
	now := s.now().Add(24 * time.Hour)
	s.Now = func() time.Time { return now }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, err := s.Runs(1, j.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("missed coalescing: %+v %v", runs, err)
	}
	got, _ := s.Job(1, j.ID)
	if got.NextRun != now.Add(time.Minute).Unix() {
		t.Fatalf("wrong next: %+v", got)
	}
	now = now.Add(time.Minute)
	if err = s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, _ = s.Runs(1, j.ID)
	got, _ = s.Job(1, j.ID)
	if len(runs) != 1 || got.LastError != "Skipped overlapping run" || got.NextRun <= now.Unix() {
		t.Fatalf("overlap: %+v %+v", runs, got)
	}
	if _, err = s.Conductor.Repo.Manage(1, []int{runs[0].TaskID}, "release"); err != nil {
		t.Fatal(err)
	}
	s.AI.Probe = func(context.Context, string) harness.EngineHealth {
		return harness.EngineHealth{Error: "Login required"}
	}
	now = now.Add(time.Minute)
	if err = s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Job(1, j.ID)
	runs, _ = s.Runs(1, j.ID)
	if len(runs) != 1 || !strings.Contains(got.LastError, "Login required") || got.NextRun <= now.Unix() {
		t.Fatalf("failure policy: %+v %+v", runs, got)
	}
}
func TestScheduleTimezoneAndDST(t *testing.T) {
	parsed, err := schedule("0 9 * * *", "America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ now, want string }{{"2026-03-07T16:00:00Z", "2026-03-08T14:00:00Z"}, {"2026-10-31T16:00:00Z", "2026-11-01T15:00:00Z"}} {
		now, _ := time.Parse(time.RFC3339, c.now)
		if got := parsed.Next(now).UTC().Format(time.RFC3339); got != c.want {
			t.Fatalf("%s != %s", got, c.want)
		}
	}
	for _, c := range [][2]string{{"* * * * * *", "UTC"}, {"* * * * *", "unknown"}, {"@every 1m", "UTC"}, {"99 * * * *", "UTC"}} {
		if _, err := schedule(c[0], c[1]); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid accepted: %+v %v", c, err)
		}
	}
}

type engineFunc func(context.Context, harness.RunSpec) (harness.RunResult, error)

func (f engineFunc) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	return f(ctx, spec)
}
func TestJobCompletesWithBoardDisabledAndRetainsBranch(t *testing.T) {
	t.Run("manual", func(t *testing.T) { jobCompletes(t, false) })
	t.Run("scheduled", func(t *testing.T) { jobCompletes(t, true) })
}
func jobCompletes(t *testing.T, scheduled bool) {
	s := fixture(t)
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	if _, err := s.DB.Exec("UPDATE project SET repoPath=? WHERE id=1", repo); err != nil {
		t.Fatal(err)
	}
	settings, raw, _ := s.Conductor.Repo.Settings(1)
	settings.UseRepository = true
	if err := s.Conductor.Repo.SaveSettings(1, settings, raw); err != nil {
		t.Fatal(err)
	}
	_, j := makeJob(t, s, scheduled)
	var taskID int
	var err error
	if scheduled {
		now := s.now().Add(time.Minute)
		s.Now = func() time.Time { return now }
		err = s.Tick(context.Background())
		runs, runErr := s.Runs(1, j.ID)
		if runErr != nil || len(runs) != 1 {
			t.Fatalf("scheduled creation: %+v %v", runs, runErr)
		}
		taskID = runs[0].TaskID
	} else {
		taskID, err = s.Fire(context.Background(), 1, j.ID, "manual", "run")
	}
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	h := engineFunc(func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		calls++
		if err := spec.OnSession("test-session", "test-turn"); err != nil {
			t.Fatal(err)
		}
		if !spec.Request.Conductor.Independent || spec.Request.Conductor.TaskAgent.ID != 11 || spec.Request.Model != "gpt-6-astra" {
			t.Fatalf("wrong contract: %+v", spec.Request)
		}
		task, _ := s.AI.Tasks.FindOneWithProject(taskID)
		if spec.Request.Conductor.Phase == "planning" {
			if task.Stage != 2 {
				t.Fatal("missing planning transition")
			}
			return harness.RunResult{Output: `{"ready":true,"context":"Audit the repo","missingContext":""}`}, nil
		}
		if task.Stage != 3 {
			t.Fatal("missing doing transition")
		}
		if err := os.WriteFile(filepath.Join(spec.WorkDir, "audit.md"), []byte("Audit results\n"), 0600); err != nil {
			t.Fatal(err)
		}
		return harness.RunResult{Output: `{"completed":true,"summary":"Audit ready for review","blocker":""}`, SessionID: "test-session", TurnID: "test-turn"}, nil
	})
	queue := &services.AgentJobService{JobRepo: s.AI.Jobs, RunRepo: s.Conductor.Runs}
	w := worker.New(queue, h)
	w.Conductor = s.Conductor
	for i := 0; i < 3; i++ {
		if _, err = w.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	states, _ := s.Conductor.Repo.Tasks(1)
	task, _ := s.AI.Tasks.FindOneWithProject(taskID)
	if len(states) != 1 || states[0].State != "completed" || task.Stage != 4 || !strings.Contains(task.Body, "Audit ready for review") || calls != 2 {
		t.Fatalf("bad completion: %+v %+v calls=%d", states, task, calls)
	}
	settings, _, _ = s.Conductor.Repo.Settings(1)
	if settings.Enabled {
		t.Fatal("board toggle changed")
	}
	runsByTask, err := s.AI.Jobs.FindByTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range runsByTask {
		runs, err := s.Conductor.Runs.ListByJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if job.Request.Conductor.Phase == "working" && run.Artifacts.Branch != "" {
				found = true
				if !strings.Contains(run.Artifacts.Diff, "Audit results") || run.Artifacts.SessionID != "test-session" {
					t.Fatalf("lost artifacts: %+v", run.Artifacts)
				}
			}
		}
	}
	if !found {
		t.Fatal("no implementation branch retained")
	}
	history, _ := s.Runs(1, j.ID)
	if len(history) != 1 || history[0].State != "completed" || history[0].TaskID != taskID {
		t.Fatalf("history: %+v", history)
	}
	second, err := s.Fire(context.Background(), 1, j.ID, "manual", "next")
	if err != nil || second == taskID {
		t.Fatalf("next run %d %v", second, err)
	}
}

func TestSchedulerLoopCreatesDueTaskAndStops(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, true)
	now := s.now().Add(time.Minute)
	s.Now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx, 5*time.Millisecond) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("scheduler never fired")
		case <-poll.C:
			runs, err := s.Runs(1, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) == 1 {
				cancel()
				select {
				case <-done:
					return
				case <-deadline.C:
					t.Fatal("scheduler did not stop")
				}
			}
		}
	}
}

func TestIndependentRecheckKeepsAgentAndPrompt(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, false)
	id, err := s.Fire(context.Background(), 1, j.ID, "manual", "first")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	h := engineFunc(func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		calls++
		if !spec.Request.Conductor.Independent || spec.Request.Conductor.TaskAgent.ID != 11 {
			t.Fatal("lost independent contract")
		}
		if spec.Request.Conductor.Phase == "planning" {
			if !strings.Contains(spec.Request.Instruction, "Report concrete findings") {
				t.Fatal("lost job prompt")
			}
			if calls == 1 {
				return harness.RunResult{Output: `{"ready":false,"context":"Need context","missingContext":"Which repo?"}`}, nil
			}
			return harness.RunResult{Output: `{"ready":true,"context":"Ready with supplied context","missingContext":""}`}, nil
		}
		return harness.RunResult{Output: `{"completed":true,"summary":"Finished after recheck","blocker":""}`}, nil
	})
	w := worker.New(&services.AgentJobService{JobRepo: s.AI.Jobs, RunRepo: s.Conductor.Runs}, h)
	w.Conductor = s.Conductor
	for i := 0; i < 2; i++ {
		if _, err = w.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := s.Runs(1, j.ID)
	if len(state) != 1 || state[0].State != "needs_context" {
		t.Fatalf("%+v", state)
	}
	body := `[{"type":"p","children":[{"text":"Additional context supplied"}]}]`
	if _, err = s.AI.Tasks.UpdateTaskBody(id, body); err != nil {
		t.Fatal(err)
	}
	if result, err := s.Conductor.Manage(1, []int{id}, "recheck"); err != nil || result.Success != 1 {
		t.Fatalf("recheck: %+v %v", result, err)
	}
	for i := 0; i < 3; i++ {
		if _, err = w.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = s.Runs(1, j.ID)
	if len(state) != 1 || state[0].State != "completed" || calls != 3 {
		t.Fatalf("%+v calls=%d", state, calls)
	}
}

func TestFireRevalidatesStagesAfterPreflight(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, false)
	s.AI.Probe = func(context.Context, string) harness.EngineHealth {
		if _, err := s.DB.Exec("UPDATE stage SET type='done' WHERE id=4"); err != nil {
			t.Fatal(err)
		}
		return harness.EngineHealth{Ready: true, Executable: "fake"}
	}
	if _, err := s.Fire(context.Background(), 1, j.ID, "manual", "changed"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stale stages accepted: %v", err)
	}
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM task").Scan(&n); err != nil || n != 0 {
		t.Fatalf("task leaked: %d %v", n, err)
	}
}

func TestShutdownDoesNotConsumeDueOccurrence(t *testing.T) {
	s := fixture(t)
	_, j := makeJob(t, s, true)
	now := s.now().Add(time.Minute)
	s.Now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.AI.Probe = func(context.Context, string) harness.EngineHealth {
		cancel()
		return harness.EngineHealth{Error: "stopping"}
	}
	if err := s.Tick(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	saved, err := s.Job(1, j.ID)
	if err != nil || saved.NextRun != j.NextRun || saved.LastError != "" {
		t.Fatalf("consumed: %+v %v", saved, err)
	}
}

func TestTemplateDoneStageSetsCompletionTime(t *testing.T) {
	s := fixture(t)
	tpl, _ := makeJob(t, s, false)
	if _, err := s.DB.Exec("INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(5,1,'Done','done','gray','circle',5)"); err != nil {
		t.Fatal(err)
	}
	tpl.StageID = 5
	tpl, err := s.UpdateTemplate(1, tpl.ID, tpl)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Instantiate(context.Background(), 1, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.AI.Tasks.FindOneWithProject(id)
	if err != nil || task.TimeCompleted == nil {
		t.Fatalf("not complete: %+v %v", task, err)
	}
}
