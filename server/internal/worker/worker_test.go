package worker

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	_ "modernc.org/sqlite"
)

func setupWorkerTestDB(t *testing.T) (*sql.DB, *services.AgentJobService, *services.TaskService) {
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
		`CREATE TABLE agent_job (id INTEGER PRIMARY KEY AUTOINCREMENT, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE, persona INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE, status TEXT NOT NULL, fromStage INTEGER, toStage INTEGER, claimedAt TEXT, startedAt TEXT, finishedAt TEXT, attempts INTEGER NOT NULL DEFAULT 0, error TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE agent_run (id INTEGER PRIMARY KEY AUTOINCREMENT, job INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE, output TEXT, summary TEXT, exitCode INTEGER, usageJson TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), artifactJson TEXT NOT NULL DEFAULT '{}' );`,
		`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`,
	}
	for i, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create table %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'wf');`); err != nil {
		t.Fatalf("seed wf: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, type, color, icon, position) VALUES (10, 1, 'open', 'open', 'gray', 'circle', 1), (20, 1, 'doing', 'doing', 'gray', 'circle', 2);`); err != nil {
		t.Fatalf("seed stage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model, allowed_tools) VALUES (1, 'researcher', 'opencode', 'opencode-go/muse-spark-1.2-contributor', '[]');`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (20, 1);`); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_type (id, name, isDefault) VALUES (1, 'Task', 1);`); err != nil {
		t.Fatalf("seed tt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, workflow, name) VALUES (1, 1, 'proj');`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1, 1, 'cl', 1);`); err != nil {
		t.Fatalf("seed cl: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, type, name, body) VALUES (1, 1, 20, 1, 'task one', '[{"type":"p","children":[{"text":"hello"}]}]'), (2, 1, 20, 1, 'task two', '[]'), (3, 1, 20, 1, 'task three', '[]');`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	taskRepo := &repos.TaskRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}

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
	}
	return db, jobSvc, taskSvc
}

type failingHarness struct {
	err error
}

func (f *failingHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	return harness.RunResult{}, f.err
}

type nonZeroHarness struct{}

func (n *nonZeroHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	return harness.RunResult{
		Output:    "# failed output for task " + spec.TaskName,
		Summary:   "failed",
		ExitCode:  2,
		UsageJson: `{"tokens":1}`,
	}, nil
}

func TestRunOnce_ClaimsAndCompletes(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	shim := harness.NewReadOnlyShim()
	w := New(jobSvc, taskSvc, shim)

	from := 10
	to := 20
	if _, err := jobSvc.Enqueue(1, 1, &from, &to); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !did {
		t.Fatal("RunOnce returned false; want true (job processed)")
	}

	job, err := jobSvc.Get(1)
	if err != nil {
		t.Fatalf("Get job: %v", err)
	}
	if job.Status != models.AgentJobStatusCompleted {
		t.Fatalf("job status = %q; want completed", job.Status)
	}
	if job.FinishedAt == nil {
		t.Fatal("FinishedAt nil after complete")
	}
	runs, err := jobSvc.RunRepo.ListByJob(job.ID)
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d; want 1", len(runs))
	}
	if !strings.Contains(runs[0].Output, "# Research notes for task 1") {
		t.Fatalf("shim output not persisted: %q", runs[0].Output)
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 0 {
		t.Fatalf("ExitCode = %v; want 0", runs[0].ExitCode)
	}
	// No stage move in Phase 3
	var stage int
	db := jobSvc.JobRepo.DB
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 20 {
		t.Fatalf("stage = %d; want 20 (no move in Phase 3)", stage)
	}
}

func TestRunOnce_NoPendingReturnsFalse(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if did {
		t.Fatal("RunOnce with no pending returned true; want false")
	}
}

func TestRunOnce_ShimsOutputPersisted(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	shim := harness.NewReadOnlyShim()
	w := New(jobSvc, taskSvc, shim)
	to := 20
	from := 10
	job, err := jobSvc.Enqueue(2, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	runs, _ := jobSvc.RunRepo.ListByJob(job.ID)
	if len(runs) != 1 {
		t.Fatalf("runs %d", len(runs))
	}
	out := runs[0].Output
	if !strings.Contains(out, "read via `search_tasks`") {
		t.Fatalf("missing shim blurb: %q", out)
	}
	if !strings.Contains(out, "Persona ID") {
		t.Fatalf("missing persona id: %q", out)
	}
}

func TestRunOnce_TwoWorkersDoNotDoubleClaim(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	if _, err := jobSvc.Enqueue(1, 1, &from, &to); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	shim := harness.NewReadOnlyShim()
	w1 := New(jobSvc, taskSvc, shim)
	w2 := New(jobSvc, taskSvc, shim)

	var wg sync.WaitGroup
	results := make([]bool, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0], errs[0] = w1.RunOnce(context.Background()) }()
	go func() { defer wg.Done(); results[1], errs[1] = w2.RunOnce(context.Background()) }()
	wg.Wait()

	for i, e := range errs {
		if e != nil && !errors.Is(e, context.Canceled) {
			// One worker may get ErrInvalidJobStatus if race on MarkRunning — acceptable
			// But our implementation serializes via ClaimNext atomic, so one returns false
			t.Logf("worker %d err: %v", i, e)
		}
	}
	claimed := 0
	for _, did := range results {
		if did {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed workers = %d; want exactly 1, results %v errs %v", claimed, results, errs)
	}
	// Verify exactly one completed job
	var completed int
	if err := jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status = 'completed';`).Scan(&completed); err != nil {
		t.Fatalf("count completed: %v", err)
	}
	if completed != 1 {
		t.Fatalf("completed count = %d; want 1", completed)
	}
	var runningOrClaimed int
	if err := jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status IN ('running','claimed');`).Scan(&runningOrClaimed); err != nil {
		t.Fatalf("count running: %v", err)
	}
	if runningOrClaimed != 0 {
		t.Fatalf("running/claimed left = %d; want 0", runningOrClaimed)
	}
}

func TestWorker_StartRecoversStale(t *testing.T) {
	db, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := jobSvc.ClaimNext()
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext: %v %v", err, claimed)
	}
	if claimed.ID != job.ID {
		t.Fatalf("claimed id mismatch")
	}
	old := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE agent_job SET claimedAt = ? WHERE id = ?;`, old, job.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	// Verify stale
	var status string
	if err := db.QueryRow(`SELECT status FROM agent_job WHERE id = ?;`, job.ID).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != models.AgentJobStatusClaimed {
		t.Fatalf("status %q", status)
	}

	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	var after string
	if err := db.QueryRow(`SELECT status FROM agent_job WHERE id = ?;`, job.ID).Scan(&after); err != nil {
		t.Fatalf("after query: %v", err)
	}
	if after != models.AgentJobStatusPending {
		t.Fatalf("after Start status = %q; want pending (stale recovered)", after)
	}

	// Ticker should then claim and complete it within next interval
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var s string
		_ = db.QueryRow(`SELECT status FROM agent_job WHERE id = ?;`, job.ID).Scan(&s)
		if s == models.AgentJobStatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var final string
	if err := db.QueryRow(`SELECT status FROM agent_job WHERE id = ?;`, job.ID).Scan(&final); err != nil {
		t.Fatalf("final query: %v", err)
	}
	if final != models.AgentJobStatusCompleted {
		t.Fatalf("final status = %q; want completed after ticker", final)
	}
}

func TestRunOnce_FailingHarnessMarksFailed(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	w := New(jobSvc, taskSvc, &failingHarness{err: errors.New("harness boom")})
	did, herr := w.RunOnce(context.Background())
	if herr == nil {
		t.Fatal("expected harness error, got nil")
	}
	if !did {
		t.Fatal("did false for failing harness; want true")
	}
	found, _ := jobSvc.Get(job.ID)
	if found.Status != models.AgentJobStatusFailed {
		t.Fatalf("status = %q; want failed", found.Status)
	}
	if found.Error == "" || !strings.Contains(found.Error, "harness boom") {
		t.Fatalf("error field = %q; want contains 'harness boom'", found.Error)
	}
	if found.FinishedAt == nil {
		t.Fatal("FinishedAt nil after fail")
	}
	runs, _ := jobSvc.RunRepo.ListByJob(job.ID)
	if len(runs) != 0 {
		t.Fatalf("runs after failing harness = %d; want 0 (fail path does not create run)", len(runs))
	}
}

func TestRunOnce_NonZeroExitMarksFailed(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	w := New(jobSvc, taskSvc, &nonZeroHarness{})
	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !did {
		t.Fatal("did false")
	}
	found, _ := jobSvc.Get(job.ID)
	if found.Status != models.AgentJobStatusFailed {
		t.Fatalf("status = %q; want failed for non-zero exit", found.Status)
	}
	runs, _ := jobSvc.RunRepo.ListByJob(job.ID)
	if len(runs) != 1 {
		t.Fatalf("runs = %d; want 1", len(runs))
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 2 {
		t.Fatalf("ExitCode = %v; want 2", runs[0].ExitCode)
	}
	if !strings.Contains(runs[0].Output, "failed output") {
		t.Fatalf("output %q", runs[0].Output)
	}
}

func TestWorker_TickerProcessesSequentially(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	for i := 1; i <= 3; i++ {
		if _, err := jobSvc.Enqueue(i, 1, &from, &to); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, 20*time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status = 'completed';`).Scan(&n)
		if n == 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var completed int
	_ = jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status = 'completed';`).Scan(&completed)
	if completed != 3 {
		t.Fatalf("completed = %d; want 3 sequential via ticker", completed)
	}
	var runs int
	_ = jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_run;`).Scan(&runs)
	if runs != 3 {
		t.Fatalf("runs = %d; want 3", runs)
	}
}

func TestWorker_StartUsesTickerNotBusyLoop(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// No jobs enqueued; ensure worker does not spin — check that RunOnce would have returned false
	// and that Stop returns promptly (busy loop would delay).
	start := time.Now()
	time.Sleep(120 * time.Millisecond)
	w.Stop()
	cancel()
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond || elapsed > 800*time.Millisecond {
		t.Fatalf("elapsed %v unexpected; ticker should have ticked ~2x without busy loop", elapsed)
	}
	var n int
	_ = jobSvc.JobRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n)
	if n != 0 {
		t.Fatalf("jobs %d want 0", n)
	}
}
