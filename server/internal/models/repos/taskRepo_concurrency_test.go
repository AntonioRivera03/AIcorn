package repos

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openSharedTaskDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:task_cas_%s_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", t.Name(), time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(5)
	stmts := []string{
		`CREATE TABLE workflow (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE stage (id INTEGER PRIMARY KEY, workflow INTEGER NOT NULL REFERENCES workflow(id) ON DELETE CASCADE, name TEXT NOT NULL DEFAULT '', description TEXT, color TEXT NOT NULL DEFAULT 'gray', icon TEXT NOT NULL DEFAULT 'circle', position INTEGER NOT NULL DEFAULT 0, type TEXT NOT NULL DEFAULT 'todo', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE persona (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', system_prompt TEXT NOT NULL DEFAULT '[]', harness TEXT NOT NULL DEFAULT 'opencode', model TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor', agent TEXT NOT NULL DEFAULT '', allowed_tools TEXT NOT NULL DEFAULT '[]', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE stage_persona (stage_id INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE, persona_id INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE);`,
		`CREATE TABLE project (id INTEGER PRIMARY KEY, name TEXT, pinned BOOLEAN, workflow INTEGER REFERENCES workflow(id), defaultView TEXT, timeCreated TEXT, timeModified TEXT);`,
		`CREATE TABLE checklist (id INTEGER PRIMARY KEY, project INTEGER REFERENCES project(id), name TEXT, description TEXT, timeCreated TEXT, timeModified TEXT, isDefault BOOLEAN);`,
		`CREATE TABLE task_type (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT 'square-check', color TEXT NOT NULL DEFAULT 'gray', isDefault INTEGER NOT NULL DEFAULT 0, timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
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
	return db
}

func TestTaskRepo_CompareAndSwapStage_ConcurrentExactlyOnce(t *testing.T) {
	db := openSharedTaskDB(t)
	repo := &TaskRepo{DB: db}

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]bool, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := repo.CompareAndSwapStage(1, 10, 20)
			results[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent CAS")
	}

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d err = %v; want nil", i, err)
		}
	}
	successes := 0
	for _, ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d; want exactly 1 (got results %v)", successes, results)
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 20 {
		t.Fatalf("final stage = %d; want 20", stage)
	}
}

func TestTaskRepo_CompareAndSwapStage_ConcurrentReversed(t *testing.T) {
	db := openSharedTaskDB(t)
	// Move task to 20 first
	if _, err := db.Exec(`UPDATE task SET stage = 20 WHERE id = 1;`); err != nil {
		t.Fatalf("setup stage 20: %v", err)
	}
	repo := &TaskRepo{DB: db}

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]bool, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := repo.CompareAndSwapStage(1, 20, 10)
			results[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent CAS reversed")
	}

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d err = %v; want nil", i, err)
		}
	}
	successes := 0
	for _, ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d; want exactly 1 (got results %v)", successes, results)
	}
	var stage int
	if err := db.QueryRow(`SELECT stage FROM task WHERE id = 1;`).Scan(&stage); err != nil {
		t.Fatalf("query stage: %v", err)
	}
	if stage != 10 {
		t.Fatalf("final stage = %d; want 10", stage)
	}
}

func TestTaskRepo_CompareAndSwapStage_ConcurrentTwoTasks(t *testing.T) {
	db := openSharedTaskDB(t)
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, type, name) VALUES (2, 1, 10, 1, 'task2');`); err != nil {
		t.Fatalf("seed task2: %v", err)
	}
	repo := &TaskRepo{DB: db}

	var wg sync.WaitGroup
	wg.Add(2)
	barrier := make(chan struct{})
	results := make([]bool, 2)
	errs := make([]error, 2)
	taskIDs := []int{1, 2}
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			ok, err := repo.CompareAndSwapStage(taskIDs[idx], 10, 20)
			results[idx] = ok
			errs[idx] = err
		}(i)
	}
	close(barrier)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent two tasks")
	}

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d err = %v; want nil", i, err)
		}
		if !results[i] {
			t.Fatalf("goroutine %d ok = false; want true for distinct tasks", i)
		}
	}
	for _, id := range taskIDs {
		var stage int
		if err := db.QueryRow(`SELECT stage FROM task WHERE id = ?;`, id).Scan(&stage); err != nil {
			t.Fatalf("query stage task %d: %v", id, err)
		}
		if stage != 20 {
			t.Fatalf("task %d stage = %d; want 20", id, stage)
		}
	}
}
