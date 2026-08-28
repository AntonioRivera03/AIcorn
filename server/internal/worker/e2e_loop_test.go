package worker

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

// E2E loop verification for Task 88 — closes the demo loop end-to-end.
//
// Flow under test:
//   backlog (open) --TransitionStage--> research (doing, bound to Read-only Researcher)
//       -> enqueued (pending) -> claimed -> shim Run -> agent_run written -> task stays in stage
//
// Manual verification (curl) after `AYCORN_DB=./app.db go run ./cmd/web`:
//   1. Create persona:
//      curl -X POST localhost:8000/api/personas -H 'Content-Type: application/json' \
//        -d '{"name":"Read-only Researcher","harness":"opencode","model":"opencode-go/muse-spark-1.2-contributor","allowed_tools":["read_task","search_tasks","list_projects"]}'
//   2. Bind to stage (replace $STAGE_ID $PERSONA_ID):
//      curl -X POST localhost:8000/api/stages/$STAGE_ID/persona -H 'Content-Type: application/json' -d '{"persona_id":'$PERSONA_ID'}'
//   3. Create task in backlog stage, then transition:
//      curl -X POST localhost:8000/api/tasks/$TASK_ID/transition -H 'Content-Type: application/json' -d '{"fromStage":10,"toStage":20}'
//   4. Observe enqueued job:
//      curl localhost:8000/api/agent-jobs/$TASK_ID
//   5. Wait one ticker (10s) then re-fetch — job should be completed, runs[0].output is markdown:
//      curl localhost:8000/api/agent-jobs/$TASK_ID | jq .runs[0].output
//   6. Verify task.body unchanged (still Plate JSON '[]' / original):
//      curl localhost:8000/api/task/$TASK_ID | jq .Body
//   7. Stale recovery (simulate crash): claim a job, backdate claimedAt >5m, wait for ResetStale or restart server.

func setupE2ETestDB(t *testing.T) (*sql.DB, *services.TaskService, *services.AgentJobService, *services.PersonaService, *repos.StagePersonaRepo) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	stmts := []string{
		`CREATE TABLE workflow (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE stage (id INTEGER PRIMARY KEY, workflow INTEGER NOT NULL REFERENCES workflow(id) ON DELETE CASCADE, name TEXT NOT NULL DEFAULT '', description TEXT, color TEXT NOT NULL DEFAULT 'gray', icon TEXT NOT NULL DEFAULT 'circle', position INTEGER NOT NULL DEFAULT 0, type TEXT NOT NULL DEFAULT 'todo', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE persona (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', system_prompt TEXT NOT NULL DEFAULT '[]', harness TEXT NOT NULL DEFAULT 'opencode', model TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor', agent TEXT NOT NULL DEFAULT '', allowed_tools TEXT NOT NULL DEFAULT '[]', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE stage_persona (stage_id INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE, persona_id INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE);`,
		`CREATE TABLE project (id INTEGER PRIMARY KEY, name TEXT, pinned BOOLEAN, workflow INTEGER REFERENCES workflow(id), defaultView TEXT, timeCreated TEXT, timeModified TEXT);`,
		`CREATE TABLE checklist (id INTEGER PRIMARY KEY, project INTEGER REFERENCES project(id), name TEXT, description TEXT, timeCreated TEXT, timeModified TEXT, isDefault BOOLEAN);`,
		`CREATE TABLE task_type (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT 'square-check', color TEXT NOT NULL DEFAULT 'gray', isDefault INTEGER NOT NULL DEFAULT 0, timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE task (id INTEGER PRIMARY KEY, checklist INTEGER REFERENCES checklist(id), stage INTEGER NOT NULL REFERENCES stage(id) ON DELETE RESTRICT, type INTEGER NOT NULL REFERENCES task_type(id), name TEXT DEFAULT '', body TEXT DEFAULT '[]', timeCreated TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timePlannedStart TEXT, timePlannedEnd TEXT, hasTimePlannedStart BOOLEAN NOT NULL DEFAULT 0, hasTimePlannedEnd BOOLEAN NOT NULL DEFAULT 0, timeCompleted TEXT, assignee TEXT, priority TEXT);`,
		`CREATE TABLE agent_job (id INTEGER PRIMARY KEY AUTOINCREMENT, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE, persona INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE, status TEXT NOT NULL, fromStage INTEGER, toStage INTEGER, claimedAt TEXT, startedAt TEXT, finishedAt TEXT, attempts INTEGER NOT NULL DEFAULT 0, error TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE agent_run (id INTEGER PRIMARY KEY AUTOINCREMENT, job INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE, output TEXT, summary TEXT, exitCode INTEGER, usageJson TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`,
	}
	for i, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create table %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'demo workflow');`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	// 10 = backlog (open), 20 = research (doing) — the bound demo stage
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, type, color, icon, position) VALUES (10, 1, 'backlog', 'open', 'gray', 'circle', 1), (20, 1, 'research', 'doing', 'gray', 'circle', 2);`); err != nil {
		t.Fatalf("seed stage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_type (id, name, isDefault) VALUES (1, 'Task', 1);`); err != nil {
		t.Fatalf("seed task_type: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, workflow, name) VALUES (1, 1, 'demo project');`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1, 1, 'demo checklist', 1);`); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	personaRepo := &repos.PersonaRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	taskRepo := &repos.TaskRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}

	personaSvc := &services.PersonaService{PersonaRepo: personaRepo}
	jobSvc := &services.AgentJobService{
		JobRepo:     agentJobRepo,
		RunRepo:     agentRunRepo,
		PersonaRepo: personaRepo,
		TaskRepo:    nil,
	}
	taskSvc := &services.TaskService{
		TaskRepo:         taskRepo,
		TaskTypeRepo:     &repos.TaskTypeRepo{DB: db},
		AgentJobService:  jobSvc,
		StagePersonaRepo: stagePersonaRepo,
		PersonaRepo:      personaRepo,
	}
	return db, taskSvc, jobSvc, personaSvc, stagePersonaRepo
}

func TestE2E_TransitionEnqueueClaimRunPersistsMarkdown(t *testing.T) {
	db, taskSvc, jobSvc, personaSvc, stagePersonaRepo := setupE2ETestDB(t)

	// 1. Create read-only Research persona via PersonaService (not raw SQL) — mirrors prod seed.
	readOnlyTools := []string{"read_task", "search_tasks", "list_projects"}
	persona, err := personaSvc.Create(&models.Persona{
		Name:         "Read-only Researcher",
		Harness:      models.PersonaHarnessOpencode,
		Model:        models.PersonaModelMuseSpark12Contributor,
		AllowedTools: readOnlyTools,
		SystemPrompt: models.EmptyBody,
	})
	if err != nil {
		t.Fatalf("Create persona: %v", err)
	}
	if persona.Harness != models.PersonaHarnessOpencode {
		t.Fatalf("persona harness = %q; want opencode", persona.Harness)
	}
	if persona.Model != models.PersonaModelMuseSpark12Contributor {
		t.Fatalf("persona model = %q; want %q", persona.Model, models.PersonaModelMuseSpark12Contributor)
	}
	// Restrictive allowed_tools check
	if len(persona.AllowedTools) != 3 {
		t.Fatalf("allowed_tools len = %d; want 3 read-only", len(persona.AllowedTools))
	}
	for _, w := range []string{"update_task", "create_task", "delete_task", "write", "edit"} {
		for _, got := range persona.AllowedTools {
			if got == w {
				t.Fatalf("allowed_tools contains write tool %q; must be read-only", w)
			}
		}
	}

	// 2. Bind persona to stage 20 (research).
	if err := stagePersonaRepo.Bind(20, persona.ID); err != nil {
		t.Fatalf("Bind stage_persona: %v", err)
	}
	sp, err := stagePersonaRepo.FindByStage(20)
	if err != nil {
		t.Fatalf("FindByStage: %v", err)
	}
	if sp.PersonaID != persona.ID {
		t.Fatalf("bound persona %d != created %d", sp.PersonaID, persona.ID)
	}

	// 3. Create task in backlog stage 10 with canonical empty body (Plate JSON '[]').
	originalBody := models.EmptyBody
	res, err := db.Exec(`INSERT INTO task (checklist, stage, type, name, body) VALUES (1, 10, 1, 'demo task: research E2E', ?);`, originalBody)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	taskID64, _ := res.LastInsertId()
	taskID := int(taskID64)
	var storedBody string
	if err := db.QueryRow(`SELECT body FROM task WHERE id = ?;`, taskID).Scan(&storedBody); err != nil {
		t.Fatalf("query body: %v", err)
	}
	if storedBody != models.EmptyBody {
		t.Fatalf("task body after create = %q; want %q", storedBody, models.EmptyBody)
	}

	// 4. Transition backlog -> research. This must enqueue a pending job.
	ok, err := taskSvc.TransitionStage(taskID, 10, 20)
	if err != nil {
		t.Fatalf("TransitionStage: %v", err)
	}
	if !ok {
		t.Fatal("TransitionStage returned false; want true")
	}
	// Task must have moved to stage 20 and must stay there (human gate — no auto-advance).
	var stageAfter int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = ?;`, taskID).Scan(&stageAfter); err != nil {
		t.Fatalf("query task stage: %v", err)
	}
	if stageAfter != 20 {
		t.Fatalf("task stage after transition = %d; want 20", stageAfter)
	}
	jobs, err := jobSvc.FindByTask(taskID)
	if err != nil {
		t.Fatalf("FindByTask: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs for task = %d; want 1 pending", len(jobs))
	}
	if jobs[0].Status != models.AgentJobStatusPending {
		t.Fatalf("job status = %q; want pending after TransitionStage", jobs[0].Status)
	}
	if jobs[0].Persona != persona.ID {
		t.Fatalf("job persona %d != %d", jobs[0].Persona, persona.ID)
	}

	// 5. Worker claims and runs shim.
	shim := harness.NewReadOnlyShim()
	w := New(jobSvc, taskSvc, shim)
	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !did {
		t.Fatal("RunOnce returned false; want true (job processed)")
	}

	// 6. Verify job is completed, run output is markdown, not Plate JSON.
	completed, err := jobSvc.Get(jobs[0].ID)
	if err != nil {
		t.Fatalf("Get job: %v", err)
	}
	if completed.Status != models.AgentJobStatusCompleted {
		t.Fatalf("job status after RunOnce = %q; want completed", completed.Status)
	}
	if completed.FinishedAt == nil {
		t.Fatal("FinishedAt nil after complete")
	}
	runs, err := jobSvc.RunRepo.ListByJob(completed.ID)
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d; want 1", len(runs))
	}
	output := runs[0].Output
	if !strings.Contains(output, "# Research notes for task") {
		t.Fatalf("agent_run.output missing markdown header: %q", output)
	}
	if !strings.HasPrefix(strings.TrimSpace(output), "#") {
		t.Fatalf("agent_run.output should be markdown starting with header, got: %q", output)
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 0 {
		t.Fatalf("ExitCode = %v; want 0", runs[0].ExitCode)
	}
	if runs[0].UsageJson == "" {
		t.Fatal("UsageJson empty; shim should emit {\"shim\":true,...}")
	}

	// 7. Verify output lands in agent_run.output as markdown, NOT task.body (which stays Plate JSON).
	var bodyAfter string
	if err := db.QueryRow(`SELECT body FROM task WHERE id = ?;`, taskID).Scan(&bodyAfter); err != nil {
		t.Fatalf("query task body: %v", err)
	}
	if bodyAfter != models.EmptyBody {
		t.Fatalf("task.body mutated to %q; want still %q", bodyAfter, models.EmptyBody)
	}
	if bodyAfter == output {
		t.Fatal("task.body equals agent_run.output")
	}

	listResp, err := jobSvc.ListByTask(taskID)
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(listResp.Jobs) != 1 || len(listResp.Runs) != 1 {
		t.Fatalf("ListByTask jobs=%d runs=%d; want 1 each", len(listResp.Jobs), len(listResp.Runs))
	}
	if listResp.Runs[0].Output != output {
		t.Fatalf("ListByTask run output mismatch")
	}

	var finalStage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = ?;`, taskID).Scan(&finalStage); err != nil {
		t.Fatalf("final stage query: %v", err)
	}
	if finalStage != 20 {
		t.Fatalf("final stage = %d; want 20", finalStage)
	}

	res2, err := db.Exec(`INSERT INTO task (checklist, stage, type, name, body) VALUES (1, 10, 1, 'second demo task', ?);`, models.EmptyBody)
	if err != nil {
		t.Fatalf("insert second task: %v", err)
	}
	secondID64, _ := res2.LastInsertId()
	secondID := int(secondID64)
	if _, err := taskSvc.TransitionStage(secondID, 10, 20); err != nil {
		t.Fatalf("TransitionStage second: %v", err)
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce second: %v", err)
	}
	jobs2, _ := jobSvc.FindByTask(secondID)
	if len(jobs2) != 1 {
		t.Fatalf("second jobs len %d", len(jobs2))
	}
	runs2, _ := jobSvc.RunRepo.ListByJob(jobs2[0].ID)
	if len(runs2) != 1 {
		t.Fatalf("second runs len %d", len(runs2))
	}
	if !strings.Contains(runs2[0].Output, "read_task") || !strings.Contains(runs2[0].Output, "search_tasks") {
		t.Fatalf("shim output missing read-only tools: %q", runs2[0].Output)
	}
	if strings.Contains(runs2[0].Output, "update_task") {
		t.Fatalf("sham output unexpectedly contains write tool: %q", runs2[0].Output)
	}
}

func TestE2E_StaleRecovery_ResetStale(t *testing.T) {
	_, taskSvc, jobSvc, personaSvc, stagePersonaRepo := setupE2ETestDB(t)

	persona, err := personaSvc.Create(&models.Persona{
		Name:         "Read-only Researcher",
		Harness:      models.PersonaHarnessOpencode,
		Model:        models.PersonaModelMuseSpark12Contributor,
		AllowedTools: []string{"read_task", "search_tasks", "list_projects"},
	})
	if err != nil {
		t.Fatalf("Create persona: %v", err)
	}
	if err := stagePersonaRepo.Bind(20, persona.ID); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	res, err := jobSvc.JobRepo.DB.Exec(`INSERT INTO task (checklist, stage, type, name, body) VALUES (1, 10, 1, 'stale test task', ?);`, models.EmptyBody)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	id64, _ := res.LastInsertId()
	taskID := int(id64)
	if _, err := taskSvc.TransitionStage(taskID, 10, 20); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	jobs, _ := jobSvc.FindByTask(taskID)
	if len(jobs) != 1 {
		t.Fatalf("jobs len %d", len(jobs))
	}
	jobID := jobs[0].ID

	// Claim it (moves pending -> claimed with claimedAt = now).
	claimed, err := jobSvc.ClaimNext()
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext: %v %v", err, claimed)
	}
	if claimed.ID != jobID {
		t.Fatalf("claimed ID %d != job %d", claimed.ID, jobID)
	}
	if claimed.Status != models.AgentJobStatusClaimed {
		t.Fatalf("status %q want claimed", claimed.Status)
	}

	// Simulate crash: backdate claimedAt beyond StaleTimeout (5m).
	db := jobSvc.JobRepo.DB
	old := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE agent_job SET claimedAt = ? WHERE id = ?;`, old, jobID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// ResetStale with 5m timeout should move it back to pending with NULL claimedAt.
	n, err := jobSvc.ResetStale(5 * time.Minute)
	if err != nil {
		t.Fatalf("ResetStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResetStale affected %d; want 1", n)
	}
	after, err := jobSvc.Get(jobID)
	if err != nil {
		t.Fatalf("Get after reset: %v", err)
	}
	if after.Status != models.AgentJobStatusPending {
		t.Fatalf("status after ResetStale = %q; want pending", after.Status)
	}
	if after.ClaimedAt != nil {
		t.Fatalf("ClaimedAt after reset = %v; want nil", after.ClaimedAt)
	}

	// Re-claim and complete via Worker — must succeed after recovery.
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce after stale: %v", err)
	}
	if !did {
		t.Fatal("RunOnce after stale returned false")
	}
	final, _ := jobSvc.Get(jobID)
	if final.Status != models.AgentJobStatusCompleted {
		t.Fatalf("final status = %q; want completed after recovery", final.Status)
	}
	runs, _ := jobSvc.RunRepo.ListByJob(jobID)
	if len(runs) != 1 || !strings.Contains(runs[0].Output, "# Research notes") {
		t.Fatalf("run after recovery missing markdown: %v", runs)
	}
	// Task still stays in stage 20 — no auto-advance.
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = ?;`, taskID).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 20 {
		t.Fatalf("stage after recovery = %d; want 20", stage)
	}
}

func TestE2E_PersonaReadOnlyRestricted(t *testing.T) {
	_, _, _, personaSvc, _ := setupE2ETestDB(t)

	persona, err := personaSvc.Create(&models.Persona{
		Name:         "Read-only Researcher",
		Harness:      models.PersonaHarnessOpencode,
		AllowedTools: []string{"read_task", "search_tasks", "list_projects"},
	})
	if err != nil {
		t.Fatalf("Create persona: %v", err)
	}
	// Defaults: harness opencode, model muse-spark-1.2-contributor when empty.
	if persona.Harness != models.PersonaHarnessOpencode {
		t.Fatalf("default harness = %q; want opencode", persona.Harness)
	}
	if persona.Model != models.PersonaModelMuseSpark12Contributor {
		t.Fatalf("default model = %q; want muse-spark", persona.Model)
	}
	// Only read tools allowed.
	wantReads := map[string]bool{"read_task": true, "search_tasks": true, "list_projects": true}
	if len(persona.AllowedTools) != len(wantReads) {
		t.Fatalf("allowed_tools = %v; want %v", persona.AllowedTools, wantReads)
	}
	for _, tool := range persona.AllowedTools {
		if !wantReads[tool] {
			t.Fatalf("unexpected tool %q; only read tools permitted", tool)
		}
	}
	// Creating with model explicitly should also validate.
	p2, err := personaSvc.Create(&models.Persona{
		Name:         "Second Researcher",
		Harness:      models.PersonaHarnessOpencode,
		Model:        models.PersonaModelMuseSpark12Contributor,
		AllowedTools: []string{"read_task"},
	})
	if err != nil {
		t.Fatalf("Create second persona: %v", err)
	}
	if p2.Model != models.PersonaModelMuseSpark12Contributor {
		t.Fatalf("second model %q", p2.Model)
	}
}
