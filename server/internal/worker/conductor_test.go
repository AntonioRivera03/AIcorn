package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
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
	h := &testEngine{run: func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		output := completeDecision
		if spec.Request.Conductor.Phase == "planning" {
			output = readyDecision
		}
		return harness.RunResult{Output: output, UsageJson: "{}"}, nil
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

func TestConductorLifecyclePreservesBodyAndStopsAtReview(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		if spec.Request.Conductor.Phase == "planning" {
			conductorState(t, w, "planning", 2)
			if spec.Request.Model != "gpt-6-astra" {
				t.Fatal("wrong planner model")
			}
			return harness.RunResult{Output: readyDecision}, nil
		}
		conductorState(t, w, "working", 3)
		if spec.Request.Model != "gpt-5.6-sol" || !strings.Contains(spec.Request.TaskBody, "Conductor planning") {
			t.Fatal("worker did not get the plan or selected model")
		}
		// Concurrent human body edits during work survive the appended handoff.
		current, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
		var nodes []any
		if err := json.Unmarshal([]byte(current.Body), &nodes); err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, map[string]any{"type": "p", "children": []any{map[string]any{"text": "Human added detail"}}})
		raw, _ := json.Marshal(nodes)
		if _, err := w.Conductor.AI.Tasks.UpdateTaskBody(1, string(raw)); err != nil {
			t.Fatal(err)
		}
		return harness.RunResult{Output: completeDecision}, nil
	}
	runConductor(t, w)
	if err := w.Conductor.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	conductorState(t, w, "queued", 2)
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "completed", 4)
	task, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
	for _, text := range []string{"Keep this original requirement", `"bold":true`, "Human added detail", "Conductor handoff", "Conductor planning"} {
		if !strings.Contains(task.Body, text) {
			t.Fatalf("lost %s: %s", text, task.Body)
		}
	}
	if task.TimeCompleted != nil || task.Assignee != "AI · Task agent" {
		t.Fatalf("bad handoff: %+v", task)
	}
	untouched, _ := w.Conductor.AI.Tasks.FindOneWithProject(2)
	if untouched.Stage != 1 {
		t.Fatal("unselected task moved")
	}
	runConductor(t, w)
	if h.calls.Load() != 2 || strings.Count(task.Body, "Conductor handoff") != 1 {
		t.Fatal("work or handoff replayed")
	}
	if _, err := w.JobService.JobRepo.DB.Exec(`UPDATE task SET stage=5 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	runConductor(t, w)
	list, err := w.Conductor.Repo.Tasks(1)
	if err != nil || len(list) != 0 || h.calls.Load() != 2 {
		t.Fatal("human approval kept the review badge or restarted work")
	}
	jobs, err := w.JobService.JobRepo.FindByTask(1)
	if err != nil || len(jobs) != 2 {
		t.Fatal("human approval lost run history")
	}
}

func TestConductorMissingContextRequiresExplicitRecheck(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	h.run = func(context.Context, harness.RunSpec) (harness.RunResult, error) {
		return harness.RunResult{Output: `{"ready":false,"context":"The task is underspecified.","missingContext":"Which inputs and outputs are expected?"}`}, nil
	}
	runConductor(t, w)
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "needs_context", 2)
	if h.calls.Load() != 1 {
		t.Fatal("retried without user action")
	}
	task, _ := w.Conductor.AI.Tasks.FindOneWithProject(1)
	if !strings.Contains(task.Body, "Which inputs") {
		t.Fatal("questions not recorded in body")
	}
	if r, err := w.Conductor.Manage(1, []int{1}, "recheck"); err != nil || r.Success != 1 {
		t.Fatalf("recheck %+v %v", r, err)
	}
	runConductor(t, w)
	if h.calls.Load() != 2 {
		t.Fatal("explicit recheck not run")
	}
}

func TestConductorCanFlagAnEmptyTask(t *testing.T) {
	w, h := conductorWorker(t)
	if _, err := w.Conductor.AI.Tasks.UpdateTaskBody(1, "[]"); err != nil {
		t.Fatal(err)
	}
	sendConductor(t, w)
	h.run = func(context.Context, harness.RunSpec) (harness.RunResult, error) {
		return harness.RunResult{Output: `{"ready":false,"context":"","missingContext":"Please add acceptance criteria."}`}, nil
	}
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "needs_context", 2)
}

func TestConductorPauseSkipsQueueButAllowsManualRuns(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	if err := w.Conductor.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
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
	ct := conductorState(t, w, "planning", 2)
	job, _ := w.JobService.JobRepo.FindOne(ct.JobID)
	if job.Status != "pending" || h.calls.Load() != 1 {
		t.Fatal("pending work lost or executed")
	}
}

func TestConductorPauseAfterClaimReturnsToQueue(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	if err := w.Conductor.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, err := w.Conductor.Repo.ClaimNext()
	if err != nil {
		t.Fatal(err)
	}
	settings, previous, _ := w.Conductor.Repo.Settings(1)
	settings.Enabled = false
	if err := w.Conductor.Repo.SaveSettings(1, settings, previous); err != nil {
		t.Fatal(err)
	}
	if _, err = w.runExplicit(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	got, _ := w.JobService.JobRepo.FindOne(job.ID)
	if got.Status != "pending" || h.calls.Load() != 0 {
		t.Fatalf("pause race %+v", got)
	}
}

func TestConductorManualMoveAndReleaseWinCompletion(t *testing.T) {
	for _, action := range []string{"move", "release", "edit planning"} {
		t.Run(action, func(t *testing.T) {
			w, h := conductorWorker(t)
			sendConductor(t, w)
			h.run = func(context.Context, harness.RunSpec) (harness.RunResult, error) {
				switch action {
				case "move":
					_, err := w.JobService.JobRepo.DB.Exec(`UPDATE task SET stage=5 WHERE id=1`)
					if err != nil {
						t.Fatal(err)
					}
				case "release":
					if _, err := w.Conductor.Manage(1, []int{1}, "release"); err != nil {
						t.Fatal(err)
					}
				case "edit planning":
					if _, err := w.Conductor.AI.Tasks.UpdateTaskBody(1, `[{"type":"p","children":[{"text":"Updated requirements"}]}]`); err != nil {
						t.Fatal(err)
					}
				}
				return harness.RunResult{Output: readyDecision}, nil
			}
			runConductor(t, w)
			runConductor(t, w)
			if h.calls.Load() != 1 {
				t.Fatal("worker ran after takeover")
			}
			if action == "move" {
				conductorState(t, w, "held", 5)
			}
			if action == "edit planning" {
				conductorState(t, w, "held", 2)
			}
			if action == "release" {
				list, _ := w.Conductor.Repo.Tasks(1)
				if len(list) != 0 {
					t.Fatal("release was undone")
				}
			}
		})
	}
}

func TestConductorRestartNeverReplaysUncertainWork(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	if err := w.Conductor.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, err := w.Conductor.Repo.ClaimNext()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Conductor.BeginJob(job); err != nil {
		t.Fatal(err)
	}
	if err = w.JobService.JobRepo.InterruptInFlight(); err != nil {
		t.Fatal(err)
	}
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "failed", 2)
	if h.calls.Load() != 0 {
		t.Fatal("uncertain run replayed")
	}
}

func TestConductorFailureAndIncompleteWorkNeverReachReview(t *testing.T) {
	for _, kind := range []string{"provider", "malformed", "unauthorized assignment", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			w, h := conductorWorker(t)
			sendConductor(t, w)
			h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
				if spec.Request.Conductor.Phase == "working" {
					return harness.RunResult{Output: `{"completed":false,"summary":"Unable to finish.","blocker":"Need an example input."}`}, nil
				}
				switch kind {
				case "provider":
					return harness.RunResult{}, errors.New("provider offline")
				case "malformed":
					return harness.RunResult{Output: `{"ready":true}`}, nil
				case "unauthorized assignment":
					return harness.RunResult{Output: strings.TrimSuffix(readyDecision, "}") + `,"workerModel":"unapproved/model"}`}, nil
				default:
					return harness.RunResult{Output: readyDecision}, nil
				}
			}
			runConductor(t, w)
			runConductor(t, w)
			runConductor(t, w)
			if kind == "incomplete" {
				conductorState(t, w, "needs_context", 3)
			} else {
				conductorState(t, w, "failed", 2)
			}
		})
	}
}

func TestConductorBlockingDependenciesAndDeletedStages(t *testing.T) {
	for _, kind := range []string{"dependency", "stage"} {
		t.Run(kind, func(t *testing.T) {
			w, h := conductorWorker(t)
			sendConductor(t, w)
			runConductor(t, w)
			q := `INSERT INTO task_relationship(fromTask,toTask,relationshipType) VALUES(2,1,1)`
			if kind == "stage" {
				q = `DELETE FROM stage WHERE id=4`
			}
			if _, err := w.JobService.JobRepo.DB.Exec(q); err != nil {
				t.Fatal(err)
			}
			runConductor(t, w)
			if h.calls.Load() != 1 {
				t.Fatal("ineligible work executed")
			}
			if kind == "dependency" {
				conductorState(t, w, "needs_context", 2)
			} else {
				conductorState(t, w, "held", 2)
			}
		})
	}
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

func TestConductorFreezesSelectedCustomAgentsForCycle(t *testing.T) {
	w, h := conductorWorker(t)
	sendConductor(t, w)
	h.run = func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		if spec.Request.Engine != "codex" {
			t.Fatal("wrong engine")
		}
		if spec.Request.Conductor.Phase == "planning" {
			if spec.Request.AgentID != 10 || spec.Request.PresetName != "Conductor agent" || !strings.Contains(spec.Request.SystemPrompt, "Plan carefully") {
				t.Fatalf("missing conductor agent: %+v", spec.Request)
			}
			// An edit or deletion after planning starts cannot change the assigned agent.
			if _, err := w.Conductor.Repo.DB.Exec(`DELETE FROM persona WHERE id=11`); err != nil {
				t.Fatal(err)
			}
			return harness.RunResult{Output: readyDecision}, nil
		}
		if spec.Request.AgentID != 11 || spec.Request.Model != "gpt-5.6-sol" || spec.Request.PresetName != "Task agent" || !strings.Contains(spec.Request.SystemPrompt, "Implement carefully") {
			t.Fatalf("lost frozen task agent: %+v", spec.Request)
		}
		return harness.RunResult{Output: completeDecision}, nil
	}
	runConductor(t, w)
	runConductor(t, w)
	runConductor(t, w)
	conductorState(t, w, "completed", 4)
	board, err := w.Conductor.Board(1)
	if err != nil || board.ConfigurationError == "" {
		t.Fatal("missing deleted-agent configuration warning", err)
	}
	// Pausing must work even if a selected agent was removed.
	if _, err = w.Conductor.UpdateSettings(context.Background(), 1, map[string]json.RawMessage{"enabled": json.RawMessage(`false`)}); err != nil {
		t.Fatal(err)
	}
}
