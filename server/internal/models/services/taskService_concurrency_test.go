package services

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	_ "modernc.org/sqlite"
)

func openSharedTransitionDB(t *testing.T) (*sql.DB, *TaskService) {
	t.Helper()
	dsn := fmt.Sprintf("file:task_svc_cas_%s_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", t.Name(), time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(5)
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
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, type, name) VALUES (1, 1, 10, 1, 'task1');`); err != nil {
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

func TestTaskService_TransitionStage_ConcurrentExactlyOnce_TwoTransitions(t *testing.T) {
	db, svc := openSharedTransitionDB(t)

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	oks := make([]bool, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := svc.TransitionStage(1, 10, 20)
			oks[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent TransitionStage bound")
	}

	successes := 0
	conflicts := 0
	for i := range errs {
		if errs[i] == nil && oks[i] {
			successes++
		} else if errors.Is(errs[i], ErrStageConflict) && !oks[i] {
			conflicts++
		} else {
			t.Fatalf("goroutine %d: ok=%v err=%v; want either success or ErrStageConflict", i, oks[i], errs[i])
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d; want exactly 1 (oks %v errs %v)", successes, oks, errs)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d; want exactly 1 (oks %v errs %v)", conflicts, oks, errs)
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 20 {
		t.Fatalf("final stage = %d; want 20", stage)
	}
	var assignee string
	if err := db.QueryRow(`SELECT COALESCE(assignee,'') FROM task WHERE id = 1;`).Scan(&assignee); err != nil {
		t.Fatalf("query assignee: %v", err)
	}
	if assignee != "" {
		t.Fatalf("assignee = %q; want unchanged owner", assignee)
	}
	if c := countPendingForTask(t, db, 1); c != 0 {
		t.Fatalf("pending jobs = %d; want 0 (no implicit enqueue)", c)
	}
}

func TestTaskService_TransitionStage_ConcurrentExactlyOnce_Unbound(t *testing.T) {
	db, svc := openSharedTransitionDB(t)

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	oks := make([]bool, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := svc.TransitionStage(1, 10, 30)
			oks[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent TransitionStage unbound")
	}

	successes := 0
	conflicts := 0
	for i := range errs {
		if errs[i] == nil && oks[i] {
			successes++
		} else if errors.Is(errs[i], ErrStageConflict) && !oks[i] {
			conflicts++
		} else {
			t.Fatalf("goroutine %d: ok=%v err=%v", i, oks[i], errs[i])
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d; want 1 (oks %v errs %v)", successes, oks, errs)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d; want 1", conflicts)
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 30 {
		t.Fatalf("final stage = %d; want 30", stage)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_job;`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 0 {
		t.Fatalf("jobs after unbound concurrent = %d; want 0", n)
	}
}

func TestTaskService_TransitionStage_Concurrent_ErrStageConflict_MapsTo409(t *testing.T) {
	_, svc := openSharedTransitionDB(t)

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	oks := make([]bool, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := svc.TransitionStage(1, 10, 20)
			oks[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent 409 transition")
	}

	// Find the conflict error
	var conflictErr error
	for i, err := range errs {
		if !oks[i] && err != nil {
			conflictErr = err
		}
	}
	if conflictErr == nil {
		t.Fatalf("expected one ErrStageConflict, got errs %v oks %v", errs, oks)
	}
	if !errors.Is(conflictErr, ErrStageConflict) {
		t.Fatalf("conflict err = %v; want ErrStageConflict via errors.Is", conflictErr)
	}
	// httpStatusForError lives in package main; we cannot import it here without an import cycle.
	// Instead assert the documented mapping inline: ErrStageConflict should map to 409.
	// We verify the sentinel is distinct from other conflict sentinels and that the service
	// returned it correctly. A handler-level integration test (taskHandler_concurrency_test.go)
	// exercises the real httpStatusForError path via httptest.
	// Direct mapping check: simulate the httpStatusForError switch for the conflict case.
	status := httpStatusForServicesErr(conflictErr)
	if status != 409 {
		t.Fatalf("http status for ErrStageConflict = %d; want 409", status)
	}
	// Also confirm the stale fromStage path returns 409: second call with stale from should still be ErrStageConflict
	_, err := svc.TransitionStage(1, 10, 20)
	if !errors.Is(err, ErrStageConflict) {
		t.Fatalf("stale TransitionStage err = %v; want ErrStageConflict after raced move", err)
	}
	if s := httpStatusForServicesErr(err); s != 409 {
		t.Fatalf("stale http status = %d; want 409", s)
	}
}

// httpStatusForServicesErr mirrors server/cmd/web/http.go httpStatusForError for the
// ErrStageConflict -> 409 branch without importing package main. It exists solely to
// assert the documented mapping without creating an import cycle.
func httpStatusForServicesErr(err error) int {
	switch {
	case errors.Is(err, ErrStageConflict),
		errors.Is(err, ErrWorkflowInUse),
		errors.Is(err, ErrDuplicateRelationship),
		errors.Is(err, ErrInvalidJobStatus),
		errors.Is(err, ErrJobStatusConflict),
		errors.Is(err, ErrNoPersonaBound),
		errors.Is(err, ErrJobAlreadyPending):
		return 409
	default:
		return 500
	}
}
