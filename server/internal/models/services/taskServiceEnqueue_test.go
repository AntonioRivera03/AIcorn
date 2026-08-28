package services

import (
	"database/sql"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	_ "modernc.org/sqlite"
)

func setupEnqueueTestDB(t *testing.T) (*sql.DB, *TaskService) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Minimal schema for enqueue tests
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
	// Seed base data
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'wf');`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, type, color, icon, position) VALUES (10, 1, 'open', 'open', 'gray', 'circle', 1), (20, 1, 'todo', 'todo', 'gray', 'circle', 2), (30, 1, 'doing', 'doing', 'gray', 'circle', 3);`); err != nil {
		t.Fatalf("seed stage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1, 'coder', 'opencode', 'opencode-go/muse-spark-1.2-contributor');`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (20, 1);`); err != nil {
		t.Fatalf("bind stage_persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_type (id, name, isDefault) VALUES (1, 'Task', 1);`); err != nil {
		t.Fatalf("seed task_type: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, workflow, name) VALUES (1, 1, 'proj');`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1, 1, 'cl', 1);`); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, type, name) VALUES (1, 1, 10, 1, 'task1'), (2, 1, 10, 1, 'task2'), (3, 1, 10, 1, 'task3');`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	taskRepo := &repos.TaskRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}
	agentJobService := &AgentJobService{
		JobRepo:     agentJobRepo,
		RunRepo:     agentRunRepo,
		PersonaRepo: personaRepo,
		TaskRepo:    nil,
	}
	svc := &TaskService{
		TaskRepo:         taskRepo,
		StagePersonaRepo: stagePersonaRepo,
		AgentJobService:  agentJobService,
	}
	return db, svc
}

func countPendingForTask(t *testing.T, db *sql.DB, taskID int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE task = ? AND status = 'pending';`, taskID).Scan(&n); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	return n
}

func TestTransitionStage_EnqueuesWhenBound(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	ok, err := svc.TransitionStage(1, 10, 20)
	if err != nil {
		t.Fatalf("TransitionStage: %v", err)
	}
	if !ok {
		t.Fatal("expected ok true")
	}
	if c := countPendingForTask(t, db, 1); c != 1 {
		t.Fatalf("pending jobs for task 1 = %d; want 1", c)
	}
	var personaID, fromStage, toStage int
	var status string
	if err := db.QueryRow(`SELECT persona, status, fromStage, toStage FROM agent_job WHERE task = 1;`).Scan(&personaID, &status, &fromStage, &toStage); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if personaID != 1 || status != models.AgentJobStatusPending || fromStage != 10 || toStage != 20 {
		t.Fatalf("job row mismatch persona=%d status=%q from=%d to=%d", personaID, status, fromStage, toStage)
	}
}

func TestTransitionStage_NoJobWhenUnbound(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	ok, err := svc.TransitionStage(1, 10, 30)
	if err != nil {
		t.Fatalf("TransitionStage: %v", err)
	}
	if !ok {
		t.Fatal("expected ok true")
	}
	if c := countPendingForTask(t, db, 1); c != 0 {
		t.Fatalf("pending jobs for unbound dest = %d; want 0", c)
	}
	// Task should still have moved
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query task stage: %v", err)
	}
	if stage != 30 {
		t.Fatalf("task stage = %d; want 30", stage)
	}
}

func TestTransitionStage_NoJobOnConflict(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	// Task 1 is in stage 10, so fromStage 30 is wrong -> conflict
	ok, err := svc.TransitionStage(1, 30, 20)
	if err == nil {
		t.Fatal("expected ErrStageConflict")
	}
	if ok {
		t.Fatal("ok should be false on conflict")
	}
	if c := countPendingForTask(t, db, 1); c != 0 {
		t.Fatalf("pending jobs after conflict = %d; want 0", c)
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 10 {
		t.Fatalf("task stage after conflict = %d; want remains 10", stage)
	}
}

func TestBulkUpdate_EnqueuesForBoundStage(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	result, err := svc.BulkUpdate([]int{1, 2, 3}, map[string]any{"Stage": 20})
	if err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	if result.Success != 3 {
		t.Fatalf("BulkUpdate success = %d; want 3", result.Success)
	}
	for _, id := range []int{1, 2, 3} {
		if c := countPendingForTask(t, db, id); c != 1 {
			t.Fatalf("task %d pending = %d; want 1", id, c)
		}
		var toStage int
		if err := db.QueryRow(`SELECT toStage FROM agent_job WHERE task = ?;`, id).Scan(&toStage); err != nil {
			t.Fatalf("query toStage task %d: %v", id, err)
		}
		if toStage != 20 {
			t.Fatalf("task %d toStage = %d; want 20", id, toStage)
		}
	}
}

func TestBulkUpdate_NoJobWhenUnbound(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	result, err := svc.BulkUpdate([]int{1, 2}, map[string]any{"Stage": 30})
	if err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	if result.Success != 2 {
		t.Fatalf("success = %d; want 2", result.Success)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after unbound bulk = %d; want 0", n)
	}
}

func TestBulkUpdate_NoJobForNonStageEdit(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	result, err := svc.BulkUpdate([]int{1, 2}, map[string]any{"Priority": "High"})
	if err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	if result.Success != 2 {
		t.Fatalf("success = %d; want 2", result.Success)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after priority-only bulk = %d; want 0", n)
	}
	// Verify priority actually changed (not silently skipped)
	var p string
	if err := db.QueryRow(`SELECT priority FROM task WHERE id = 1;`).Scan(&p); err != nil {
		t.Fatalf("query priority: %v", err)
	}
	if p != "High" {
		t.Fatalf("priority = %q; want High", p)
	}
}

func TestBulkUpdate_StageFloat64FromJSON(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	// JSON numbers decode as float64; ensure BulkUpdate handles it
	result, err := svc.BulkUpdate([]int{1}, map[string]any{"Stage": float64(20)})
	if err != nil {
		t.Fatalf("BulkUpdate float64: %v", err)
	}
	if result.Success != 1 {
		t.Fatalf("success = %d; want 1", result.Success)
	}
	if c := countPendingForTask(t, db, 1); c != 1 {
		t.Fatalf("pending after float64 stage = %d; want 1", c)
	}
}

func TestTransitionStage_AtomicRollbackOnEnqueueFailure(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF;`); err != nil {
		t.Fatalf("pragma off: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM stage_persona WHERE stage_id = 20;`); err != nil {
		t.Fatalf("delete binding: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (20, 999);`); err != nil {
		t.Fatalf("insert invalid binding: %v", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		t.Fatalf("pragma on: %v", err)
	}
	_, err := svc.TransitionStage(1, 10, 20)
	if err == nil {
		t.Fatal("TransitionStage should fail when enqueue fails atomically")
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 10 {
		t.Fatalf("stage = %d; want 10 (rolled back) after atomic failure", stage)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after atomic failure = %d; want 0", n)
	}
}

func TestBulkUpdate_AtomicRollbackOnEnqueueFailure(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF;`); err != nil {
		t.Fatalf("pragma off: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM stage_persona WHERE stage_id = 20;`); err != nil {
		t.Fatalf("delete binding: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (20, 999);`); err != nil {
		t.Fatalf("insert invalid binding: %v", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		t.Fatalf("pragma on: %v", err)
	}
	_, err := svc.BulkUpdate([]int{1, 2}, map[string]any{"Stage": 20})
	if err == nil {
		t.Fatal("BulkUpdate should fail atomically when bulk enqueue fails")
	}
	for _, id := range []int{1, 2} {
		var stage int
		if err := db.QueryRow(`SELECT stage FROM task WHERE id = ?;`, id).Scan(&stage); err != nil {
			t.Fatalf("query stage %d: %v", id, err)
		}
		if stage != 10 {
			t.Fatalf("task %d stage = %d; want 10 rolled back", id, stage)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after bulk atomic failure = %d; want 0", n)
	}
}

func TestBulkUpdate_BulkResult_SkippedNonExistent(t *testing.T) {
	db, svc := setupEnqueueTestDB(t)
	result, err := svc.BulkUpdate([]int{1, 999, 2, 999}, map[string]any{"Priority": "Low"})
	if err != nil {
		t.Fatalf("BulkUpdate: %v", err)
	}
	if result.Success != 2 {
		t.Fatalf("success = %d; want 2", result.Success)
	}
	if result.Skipped != 1 {
		t.Fatalf("skipped = %d; want 1 (deduped 999 non-existent)", result.Skipped)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after non-stage bulk = %d; want 0", n)
	}
}

func TestComplete_AtomicDuplicateRunPrevention(t *testing.T) {
	db := setupAgentJobServiceTestDB(t)
	svc := &AgentJobService{
		JobRepo:     &repos.AgentJobRepo{DB: db},
		RunRepo:     &repos.AgentRunRepo{DB: db},
		PersonaRepo: &repos.PersonaRepo{DB: db},
		TaskRepo:    nil,
	}
	job, err := svc.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := svc.ClaimNext(); err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	exit := 0
	if err := svc.Complete(job.ID, "out", "sum", &exit, ""); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	var runs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_run WHERE job = ?;`, job.ID).Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs after first complete = %d; want 1", runs)
	}
	// Second complete must fail and not create duplicate run.
	exit2 := 0
	err = svc.Complete(job.ID, "out2", "sum2", &exit2, "")
	if err != ErrInvalidJobStatus {
		t.Fatalf("second Complete err = %v; want ErrInvalidJobStatus", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_run WHERE job = ?;`, job.ID).Scan(&runs); err != nil {
		t.Fatalf("count runs second: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs after duplicate complete = %d; want still 1", runs)
	}
	// Pending job cannot be completed — no run inserted.
	pendingJob, err := svc.Enqueue(2, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue pending: %v", err)
	}
	err = svc.Complete(pendingJob.ID, "out", "sum", &exit, "")
	if err != ErrInvalidJobStatus {
		t.Fatalf("complete pending err = %v; want ErrInvalidJobStatus", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_run WHERE job = ?;`, pendingJob.ID).Scan(&runs); err != nil {
		t.Fatalf("count runs pending: %v", err)
	}
	if runs != 0 {
		t.Fatalf("runs for pending job = %d; want 0 (atomic, no orphan)", runs)
	}
}
