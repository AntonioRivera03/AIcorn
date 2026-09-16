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
		`CREATE TABLE persona (builtin_role TEXT NOT NULL DEFAULT '',id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', system_prompt TEXT NOT NULL DEFAULT '[]', harness TEXT NOT NULL DEFAULT 'opencode', model TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor', agent TEXT NOT NULL DEFAULT '', allowed_tools TEXT NOT NULL DEFAULT '[]', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE stage_persona (stage_id INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE, persona_id INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE);`,
		`CREATE TABLE project (id INTEGER PRIMARY KEY, name TEXT, pinned BOOLEAN, workflow INTEGER REFERENCES workflow(id), defaultView TEXT, timeCreated TEXT, timeModified TEXT);`,
		`CREATE TABLE checklist (id INTEGER PRIMARY KEY, project INTEGER REFERENCES project(id), name TEXT, description TEXT, timeCreated TEXT, timeModified TEXT, isDefault BOOLEAN);`,
		`CREATE TABLE task_type (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT 'square-check', color TEXT NOT NULL DEFAULT 'gray', isDefault INTEGER NOT NULL DEFAULT 0, viewMode TEXT NOT NULL DEFAULT 'document', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE task (id INTEGER PRIMARY KEY, checklist INTEGER REFERENCES checklist(id), stage INTEGER NOT NULL REFERENCES stage(id) ON DELETE RESTRICT, type INTEGER NOT NULL REFERENCES task_type(id), name TEXT DEFAULT '', body TEXT DEFAULT '[]', timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP, timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP, timePlannedStart TIMESTAMP, timePlannedEnd TIMESTAMP, hasTimePlannedStart BOOLEAN NOT NULL DEFAULT 0, hasTimePlannedEnd BOOLEAN NOT NULL DEFAULT 0, timeCompleted TIMESTAMP, assignee TEXT, priority TEXT);`,
		`CREATE TABLE agent_job (id INTEGER PRIMARY KEY AUTOINCREMENT, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE, persona INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE, status TEXT NOT NULL, fromStage INTEGER, toStage INTEGER, claimedAt TEXT, startedAt TEXT, finishedAt TEXT, attempts INTEGER NOT NULL DEFAULT 0, error TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE agent_run (id INTEGER PRIMARY KEY AUTOINCREMENT, job INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE, output TEXT, summary TEXT, exitCode INTEGER, usageJson TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), artifactJson TEXT NOT NULL DEFAULT '{}' );`,
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
	taskTypeRepo := &repos.TaskTypeRepo{DB: db}
	svc := &TaskService{
		TaskRepo:         taskRepo,
		TaskTypeRepo:     taskTypeRepo,
		StagePersonaRepo: stagePersonaRepo,
		AgentJobService:  agentJobService,
		PersonaRepo:      personaRepo,
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

// Historical bindings must never influence task ownership or start work.
func TestTaskEditsNeverEnqueue(t *testing.T) {
	for _, owner := range []string{"alice", "coder", ""} {
		t.Run(owner, func(t *testing.T) {
			db, svc := setupEnqueueTestDB(t)
			if _, err := db.Exec("UPDATE task SET assignee = ?", owner); err != nil {
				t.Fatal(err)
			}
			if ok, err := svc.TransitionStage(1, 10, 20); err != nil || !ok {
				t.Fatalf("transition: %v %v", ok, err)
			}
			var got string
			if err := db.QueryRow("SELECT assignee FROM task WHERE id=1").Scan(&got); err != nil || got != owner {
				t.Fatalf("owner changed: %q %v", got, err)
			}
			if _, err := svc.TransitionStage(1, 10, 30); err != ErrStageConflict {
				t.Fatalf("stale transition: %v", err)
			}
			r, err := svc.BulkUpdate([]int{1, 2, 999, 999}, map[string]any{"Stage": float64(20)})
			if err != nil || r.Success != 2 || r.Skipped != 1 {
				t.Fatalf("bulk: %+v %v", r, err)
			}
			if err := db.QueryRow("SELECT assignee FROM task WHERE id=2").Scan(&got); err != nil || got != owner {
				t.Fatalf("bulk changed owner: %q %v", got, err)
			}
			r, err = svc.BulkUpdate([]int{1, 2}, map[string]any{"Assignee": "coder"})
			if err != nil || r.Success != 2 {
				t.Fatalf("assignment: %+v %v", r, err)
			}
			task := &models.ChecklistTask{Task: models.Task{Checklist: 1, Stage: 10, Name: "Explicit ownership", Assignee: "coder"}}
			created, err := svc.CreateChecklistTask(task)
			if err != nil {
				t.Fatal(err)
			}
			created.Assignee = "alice"
			if _, err := svc.UpdateTask(created); err != nil {
				t.Fatal(err)
			}
			created.Assignee = "coder"
			if _, err := svc.UpdateTaskProperties(created); err != nil {
				t.Fatal(err)
			}
			var jobs int
			if err := db.QueryRow("SELECT COUNT(*) FROM agent_job").Scan(&jobs); err != nil || jobs != 0 {
				t.Fatalf("unexpected execution: %d %v", jobs, err)
			}
		})
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
