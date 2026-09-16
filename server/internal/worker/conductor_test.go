package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskintent"
)

const readyDecision = `{"ready":true,"context":"Implement the specified behavior and check the acceptance criteria.","missingContext":""}`
const completeDecision = `{"completed":true,"summary":"Implemented the task. Automated tests were not available; inspect the changes.","blocker":""}`

func conductorWorker(t *testing.T) (*Worker, *testEngine) {
	t.Helper()
	s := testService(t)
	db := s.JobRepo.DB
	for _, q := range []string{
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(2,1,'Planning','todo','gray','circle',2),(3,1,'Doing','doing','gray','circle',3),(4,1,'In review','todo','gray','circle',4),(5,1,'Done','done','gray','circle',5)`,
		`UPDATE ai_settings SET model='gpt-5.6-sol'`,
		`UPDATE persona SET model='gpt-6-astra' WHERE builtin_role='conductor'`,
		`INSERT INTO persona(id,name,harness,model,system_prompt) VALUES(10,'Conductor agent','codex','gpt-6-astra','[{"type":"p","children":[{"text":"Plan carefully"}]}]'),(11,'Task agent','codex','gpt-5.6-sol','[{"type":"p","children":[{"text":"Implement carefully"}]}]')`,
		`UPDATE task SET body='[{"type":"p","children":[{"text":"Keep this original requirement","bold":true}]}]'`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ai := &services.AIService{Presets: &repos.PersonaRepo{DB: db}, Jobs: s.JobRepo, Tasks: &repos.TaskRepo{DB: db}, Projects: &repos.ProjectRepo{DB: db}, Converter: &markdown.Converter{}, Probe: func(context.Context, string) harness.EngineHealth {
		return harness.EngineHealth{Ready: true, Executable: "test-engine", Version: "test"}
	}}
	c := &services.ConductorService{Repo: &repos.ConductorRepo{DB: db}, AI: ai, Runs: s.RunRepo}
	settings := models.DefaultConductorSettings()
	settings.Enabled = true
	settings.UseRepository = false
	settings.PlanningStage = 2
	settings.WorkingStage = 3
	settings.CompletionStage = 4
	settings.ConductorAgentID = 10
	settings.TaskAgentID = 11
	if err := c.Repo.SaveSettings(1, settings, ""); err != nil {
		t.Fatal(err)
	}
	h := &testEngine{run: func(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		if spec.Request.DispatchID > 0 {
			candidates, err := c.Candidates(ctx, spec.Request.ProjectID)
			if err != nil {
				return harness.RunResult{}, err
			}
			for _, candidate := range candidates {
				if candidate.State.State == "waiting" {
					if _, err = c.StartTask(ctx, spec.Request.ProjectID, candidate.State.TaskID, spec.Request.DispatchID, "coder"); err != nil {
						return harness.RunResult{}, err
					}
				}
			}
			return harness.RunResult{Output: "Queued eligible tasks"}, nil
		}
		session := fmt.Sprintf("task-%d-session", spec.TaskID)
		if spec.Request.Chat != nil && spec.Request.Chat.SessionID != "" {
			session = spec.Request.Chat.SessionID
		}
		if spec.OnSession != nil {
			if err := spec.OnSession(session, fmt.Sprintf("turn-%d", spec.JobID)); err != nil {
				return harness.RunResult{}, err
			}
		}
		output := completeDecision
		if spec.Request.TaskSession != nil && spec.Request.TaskSession.Mode == "question" {
			output = "Here is an explanation of the existing result."
		}
		return harness.RunResult{Output: output, UsageJson: "{}", SessionID: session}, nil
	}}
	w := New(s, h)
	w.Conductor = c
	return w, h
}

func sendConductor(t *testing.T, w *Worker) {
	t.Helper()
	r, err := w.Conductor.Manage(1, []int{1}, "send")
	if err != nil || r.Success != 1 {
		t.Fatalf("send: %+v %v", r, err)
	}
}

func conductorState(t *testing.T, w *Worker, want string, stage int) models.ConductorTask {
	t.Helper()
	list, err := w.Conductor.Repo.Tasks(1)
	if err != nil || len(list) != 1 {
		t.Fatalf("tasks: %+v %v", list, err)
	}
	task, err := w.Conductor.AI.Tasks.FindOneWithProject(1)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].State != want || task.Stage != stage {
		t.Fatalf("wanted %s stage %d, got %+v stage %d", want, stage, list[0], task.Stage)
	}
	return list[0]
}

func runConductor(t *testing.T, w *Worker) {
	t.Helper()
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConductorIndependentSessionLifecycle(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	ct := conductorState(t, w, "queued", 3)
	job, err := w.JobService.JobRepo.FindOne(ct.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Request.PresetName != "Coder" || job.Request.TaskSession == nil || job.Request.Conductor.Phase != "working" || job.Request.Model != "gpt-5.6-sol" {
		t.Fatalf("not an independent coder: %+v", job.Request)
	}
	runConductor(t, w)
	conductorState(t, w, "completed", 4)
	task, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
	if !strings.Contains(task.Body, "Keep this original requirement") || !strings.Contains(task.Body, `"bold":true`) || !strings.Contains(task.Body, "Conductor handoff") || task.TimeCompleted != nil {
		t.Fatal("lost task content or skipped human review", task)
	}
	jobs, _ := w.JobService.JobRepo.FindByTask(1)
	if len(jobs) != 1 {
		t.Fatal("dispatcher created a task-owned session", len(jobs))
	}
	runs, _ := w.JobService.RunRepo.ListByJob(job.ID)
	if len(runs) != 1 || runs[0].Artifacts.SessionID != "task-1-session" {
		t.Fatal("session not retained", runs)
	}
	runConductor(t, w)
	if h.calls.Load() != 2 {
		t.Fatal("work replayed")
	}
	if _, err = w.JobService.JobRepo.DB.Exec(`UPDATE task SET stage=5 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	runConductor(t, w)
	list, _ := w.Conductor.Repo.Tasks(1)
	if len(list) != 0 {
		t.Fatal("human move did not release review")
	}
}
func TestConductorDispatcherScopeAndDuplicateStart(t *testing.T) {
	w, _ := conductorWorker(t)
	sendConductor(t, w)
	req, _, err := w.Conductor.PrepareDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	start := func(project, task int, role string) (*models.AgentJob, error) {
		return w.Conductor.StartTask(context.Background(), project, task, req.DispatchID, role)
	}
	for _, bad := range []struct {
		project, task int
		role          string
	}{{2, 1, "coder"}, {1, 2, "coder"}, {1, 1, "conductor"}, {1, 1, "unknown"}} {
		if _, err = start(bad.project, bad.task, bad.role); err == nil {
			t.Fatal("invalid dispatch accepted", bad)
		}
	}
	first, err := start(1, 1, "researcher")
	if err != nil {
		t.Fatal(err)
	}
	second, err := start(1, 1, "researcher")
	if err != nil || first.ID != second.ID {
		t.Fatal("duplicate dispatch", second, err)
	}
	if first.Request.PresetName != "Research" || first.Request.Intent != "ask" {
		t.Fatal("wrong research role")
	}
	if err = w.Conductor.Repo.FinishDispatch(req.DispatchID, "Done", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = start(1, 1, "coder"); err == nil {
		t.Fatal("finished dispatcher retained authority")
	}
}
func TestConductorPauseAfterClaimReturnsToQueue(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	job, err := w.Conductor.Repo.ClaimNext()
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	settings, old, _ := w.Conductor.Repo.Settings(1)
	settings.Enabled = false
	if err = w.Conductor.Repo.SaveSettings(1, settings, old); err != nil {
		t.Fatal(err)
	}
	if _, err = w.runExplicit(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	got, _ := w.JobService.JobRepo.FindOne(job.ID)
	if got.Status != "pending" || h.calls.Load() != 1 {
		t.Fatal("paused job executed", got)
	}
}
func TestConductorFailureNeverReachesReview(t *testing.T) {
	for _, kind := range []string{"provider", "malformed", "incomplete", "cancel", "move", "release"} {
		t.Run(kind, func(t *testing.T) {
			w, h := conductorWorker(t)
			sendConductor(t, w)
			runConductor(t, w)
			h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
				switch kind {
				case "provider":
					return harness.RunResult{}, errors.New("offline")
				case "malformed":
					return harness.RunResult{Output: `{"completed":true}`}, nil
				case "incomplete":
					return harness.RunResult{Output: `{"completed":false,"summary":"Partial","blocker":"Missing input"}`}, nil
				case "cancel":
					_, _ = w.Conductor.AI.Cancel(spec.JobID)
				case "move":
					_, _ = w.JobService.JobRepo.DB.Exec(`UPDATE task SET stage=5 WHERE id=1`)
				case "release":
					_, _ = w.Conductor.Manage(1, []int{1}, "release")
				}
				return harness.RunResult{Output: completeDecision}, nil
			}
			runConductor(t, w)
			runConductor(t, w)
			task, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
			if task.Stage == 4 {
				t.Fatal("bad run reached review")
			}
			if h.calls.Load() != 2 {
				t.Fatal("uncertain run replayed")
			}
		})
	}
}
func TestConductorQueuedContextAndDependenciesRevalidated(t *testing.T) {
	for _, kind := range []string{"body", "blocker", "stage"} {
		t.Run(kind, func(t *testing.T) {
			w, h := conductorWorker(t)
			sendConductor(t, w)
			runConductor(t, w)
			query := `UPDATE task SET body='[]' WHERE id=1`
			if kind == "blocker" {
				query = `INSERT INTO task_relationship(fromTask,toTask,relationshipType) VALUES(2,1,1)`
			}
			if kind == "stage" {
				query = `DELETE FROM stage WHERE id=4`
			}
			if _, err := w.JobService.JobRepo.DB.Exec(query); err != nil {
				t.Fatal(err)
			}
			_, _ = w.RunOnce(context.Background())
			if h.calls.Load() != 1 {
				t.Fatal("stale work executed")
			}
			task, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
			if task.Stage == 4 {
				t.Fatal("invalid work reached review")
			}
		})
	}
}
func TestConductorRestartDoesNotReplay(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	job, _ := w.Conductor.Repo.ClaimNext()
	if _, err := w.Conductor.BeginJob(job); err != nil {
		t.Fatal(err)
	}
	if err := w.JobService.JobRepo.InterruptInFlight(); err != nil {
		t.Fatal(err)
	}
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "failed", 3)
	if h.calls.Load() != 1 {
		t.Fatal("uncertain session replayed")
	}
}
func TestConductorDeferRequiresRecheck(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		return harness.RunResult{Output: "Blocked"}, w.Conductor.Repo.DeferTask(1, 1, spec.Request.DispatchID, "Missing credentials")
	}
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "needs_context", 1)
	if h.calls.Load() != 1 {
		t.Fatal("repeated paid selection")
	}
	if _, err := w.Conductor.Manage(1, []int{1}, "recheck"); err != nil {
		t.Fatal(err)
	}
	runConductor(t, w)
	if h.calls.Load() != 2 {
		t.Fatal("recheck ignored")
	}
}

type intentStub struct{ value string }

func (i intentStub) Classify(context.Context, taskintent.Context) taskintent.Decision {
	return taskintent.Decision{Intent: i.value, Reason: "Classified follow-up"}
}
func TestTaskSessionFollowupsConfirmWorkAndResume(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	runConductor(t, w)
	latest, artifacts, _ := w.Conductor.AI.Jobs.LatestChat(1)
	frozenModel := latest.Request.Model
	if _, err := w.Conductor.Repo.DB.Exec(`UPDATE persona SET model='gpt-5.5' WHERE builtin_role<>''`); err != nil {
		t.Fatal(err)
	}
	in := services.TaskMessageInput{Key: "question", Message: "Why did you choose that?", PreviousJob: latest.ID, ExpectedStage: 4, Decision: "auto"}
	w.Conductor.AI.IntentClassifier = intentStub{"question"}
	result, err := w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || result.Job == nil || result.ConfirmationRequired {
		t.Fatal(result, err)
	}
	if result.Job.Request.Chat.SessionID != artifacts.SessionID || result.Job.Request.Conductor != nil || result.Job.Request.Intent != "ask" {
		t.Fatal("question mutated workflow or lost session", result.Job)
	}
	conductorState(t, w, "completed", 4)
	if _, err = w.Conductor.AI.TaskMessage(context.Background(), 1, services.TaskMessageInput{Key: "parallel", Message: "Another", PreviousJob: latest.ID, ExpectedStage: 4, Decision: "question"}); err == nil {
		t.Fatal("concurrent turn accepted")
	}
	runConductor(t, w)
	latest, _, _ = w.Conductor.AI.Jobs.LatestChat(1)
	w.Conductor.AI.IntentClassifier = intentStub{"work"}
	in = services.TaskMessageInput{Key: "work", Message: "Add a second example", PreviousJob: latest.ID, ExpectedStage: 4, Decision: "auto"}
	result, err = w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || !result.ConfirmationRequired || result.Job != nil {
		t.Fatal(result, err)
	}
	conductorState(t, w, "completed", 4)
	in.Decision = "work"
	result, err = w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || result.Job == nil {
		t.Fatal(result, err)
	}
	conductorState(t, w, "queued", 3)
	if result.Job.Request.Model != frozenModel || result.Job.Request.Chat.SessionID != artifacts.SessionID {
		t.Fatal("work started another session")
	}
	duplicate, err := w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || duplicate.Job.ID != result.Job.ID {
		t.Fatal("duplicate follow-up", duplicate, err)
	}
	runConductor(t, w)
	conductorState(t, w, "completed", 4)
	if h.calls.Load() != 4 {
		t.Fatal("unexpected executions", h.calls.Load())
	}
}
func TestTaskSessionStaleConfirmationIsRejected(t *testing.T) {
	w, _ := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	runConductor(t, w)
	latest, _, _ := w.Conductor.AI.Jobs.LatestChat(1)
	_, _ = w.JobService.JobRepo.DB.Exec(`UPDATE task SET stage=5 WHERE id=1`)
	_, err := w.Conductor.AI.TaskMessage(context.Background(), 1, services.TaskMessageInput{Key: "stale", Message: "Do more", PreviousJob: latest.ID, ExpectedStage: 4, Decision: "work"})
	if !errors.Is(err, repos.ErrChatConflict) {
		t.Fatal("stale confirmation accepted", err)
	}
}

func TestTaskSessionCanRecoverAfterFailureBeforeThreadStart(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	successful := h.run
	h.run = func(context.Context, harness.RunSpec) (harness.RunResult, error) {
		return harness.RunResult{}, errors.New("provider unavailable")
	}
	runConductor(t, w)
	conductorState(t, w, "failed", 3)
	latest, _, err := w.Conductor.AI.Jobs.LatestChat(1)
	if err != nil {
		t.Fatal(err)
	}
	in := services.TaskMessageInput{Key: "recover", Message: "Try the task again", PreviousJob: latest.ID, ExpectedStage: 3, Decision: "auto"}
	result, err := w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || !result.ConfirmationRequired || !result.RequiresNewSession || result.Job != nil {
		t.Fatal(result, err)
	}
	in.Decision = "work"
	result, err = w.Conductor.AI.TaskMessage(context.Background(), 1, in)
	if err != nil || result.Job == nil || result.Job.Request.Chat.SessionID != "" {
		t.Fatal(result, err)
	}
	h.run = successful
	runConductor(t, w)
	conductorState(t, w, "completed", 4)
}

func TestDispatcherCannotHoldATaskReleasedAndSentAgain(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	h.run = func(context.Context, harness.RunSpec) (harness.RunResult, error) {
		if _, err := w.Conductor.Manage(1, []int{1}, "release"); err != nil {
			return harness.RunResult{}, err
		}
		if _, err := w.Conductor.Manage(1, []int{1}, "send"); err != nil {
			return harness.RunResult{}, err
		}
		return harness.RunResult{Output: "Selection finished"}, nil
	}
	runConductor(t, w)
	conductorState(t, w, "waiting", 1)
}

func TestConductorBulkIsScopedAndIdempotent(t *testing.T) {
	w, _ := conductorWorker(t)
	db := w.JobService.JobRepo.DB
	for _, q := range []string{`INSERT INTO project(id,workflow,name) VALUES(2,1,'Other')`, `INSERT INTO checklist(id,project,name) VALUES(2,2,'Other')`, `INSERT INTO task(id,checklist,stage,type,name,priority) VALUES(3,2,1,1,'Other','Medium')`, `UPDATE task SET stage=5 WHERE id=2`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r, err := w.Conductor.Manage(1, []int{1, 1, 2, 3, 999}, "send")
	if err != nil || r.Success != 1 || r.Skipped != 3 {
		t.Fatalf("bulk %+v %v", r, err)
	}
	r, err = w.Conductor.Manage(1, []int{1}, "send")
	if err != nil || r.Skipped != 1 {
		t.Fatalf("duplicate %+v %v", r, err)
	}
}

func TestConductorPauseSkipsQueueButAllowsManualRuns(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	runConductor(t, w)
	settings, previous, _ := w.Conductor.Repo.Settings(1)
	settings.Enabled = false
	if err := w.Conductor.Repo.SaveSettings(1, settings, previous); err != nil {
		t.Fatal(err)
	}
	enqueue(t, w.JobService, 2)
	h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		if spec.TaskID != 2 {
			t.Fatal("paused Conductor ran")
		}
		return harness.RunResult{Output: "Manual answer"}, nil
	}
	runConductor(t, w)
	runConductor(t, w)
	ct := conductorState(t, w, "queued", 3)
	job, _ := w.JobService.JobRepo.FindOne(ct.JobID)
	if job.Status != "pending" || h.calls.Load() != 2 {
		t.Fatal("pending work lost or executed")
	}
}
