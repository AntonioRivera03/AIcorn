package repos

import (
	"database/sql"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	_ "modernc.org/sqlite"
)

func setupAgentJobTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Minimal parent tables to satisfy FKs
	if _, err := db.Exec(`CREATE TABLE persona (builtin_role TEXT NOT NULL DEFAULT '',id INTEGER PRIMARY KEY AUTOINCREMENT);`); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE task (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE agent_job (
		    id         INTEGER PRIMARY KEY AUTOINCREMENT,
		    task       INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
		    persona    INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE,
		    status     TEXT NOT NULL,
		    fromStage  INTEGER,
		    toStage    INTEGER,
		    claimedAt  TEXT,
		    startedAt  TEXT,
		    finishedAt TEXT,
		    attempts   INTEGER NOT NULL DEFAULT 0,
		    error      TEXT,
		    createdAt  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		, requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');
	`); err != nil {
		t.Fatalf("create agent_job: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE agent_run (
		    id        INTEGER PRIMARY KEY AUTOINCREMENT,
		    job       INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE,
		    output    TEXT,
		    summary   TEXT,
		    exitCode  INTEGER,
		    usageJson TEXT,
		    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		, artifactJson TEXT NOT NULL DEFAULT '{}' );
	`); err != nil {
		t.Fatalf("create agent_run: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	// Seed required FK rows
	if _, err := db.Exec(`INSERT INTO persona (id) VALUES (1);`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id) VALUES (1), (2), (3);`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAgentJobRepo_CreateAndFindOne_ScanValidRow(t *testing.T) {
	db := setupAgentJobTestDB(t)
	repo := &AgentJobRepo{DB: db}

	from := 10
	to := 20
	job := &models.AgentJob{
		Task:      1,
		Persona:   1,
		Status:    models.AgentJobStatusPending,
		FromStage: &from,
		ToStage:   &to,
		Attempts:  0,
		Error:     "",
	}
	created, err := repo.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected inserted ID to be non-zero")
	}
	if created.Status != models.AgentJobStatusPending {
		t.Fatalf("status = %q; want pending", created.Status)
	}
	if created.FromStage == nil || *created.FromStage != from {
		t.Fatalf("FromStage = %v; want %d", created.FromStage, from)
	}
	if created.ToStage == nil || *created.ToStage != to {
		t.Fatalf("ToStage = %v; want %d", created.ToStage, to)
	}
	if created.CreatedAt == nil {
		t.Fatal("CreatedAt is nil; want parsed time")
	}
	// RFC3339 parse check: stored as UTC Z
	if created.CreatedAt.Format(time.RFC3339) == "" {
		t.Fatal("CreatedAt not RFC3339")
	}

	found, err := repo.FindOne(created.ID)
	if err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if found.ID != created.ID || found.Task != 1 || found.Persona != 1 {
		t.Fatalf("FindOne mismatch: got %+v want ID %d", found, created.ID)
	}
	if found.Error != "" {
		t.Fatalf("Error = %q; want empty", found.Error)
	}
}

func TestAgentJobRepo_NullErrorAndNullableFields(t *testing.T) {
	db := setupAgentJobTestDB(t)
	// Insert directly with NULL error and NULL nullable cols to exercise COALESCE and sql.Null handling
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, error) VALUES (1, 1, 'pending', 0, NULL);`); err != nil {
		t.Fatalf("raw insert NULL error: %v", err)
	}
	repo := &AgentJobRepo{DB: db}
	jobs, err := repo.ListByStatus(models.AgentJobStatusPending)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs; want 1", len(jobs))
	}
	j := jobs[0]
	// COALESCE(error,'') must yield empty string, not scan error
	if j.Error != "" {
		t.Fatalf("Error with NULL in DB = %q; want empty string via COALESCE", j.Error)
	}
	if j.FromStage != nil {
		t.Fatalf("FromStage = %v; want nil for NULL", j.FromStage)
	}
	if j.ToStage != nil {
		t.Fatalf("ToStage = %v; want nil", j.ToStage)
	}
	if j.ClaimedAt != nil || j.StartedAt != nil || j.FinishedAt != nil {
		t.Fatalf("expected all nullable times to be nil; got claimed=%v started=%v finished=%v", j.ClaimedAt, j.StartedAt, j.FinishedAt)
	}
	if j.CreatedAt == nil {
		t.Fatal("CreatedAt nil after NULL error insert")
	}
	// Also verify FindByTask returns same
	byTask, err := repo.FindByTask(1)
	if err != nil {
		t.Fatalf("FindByTask: %v", err)
	}
	if len(byTask) != 1 || byTask[0].Error != "" {
		t.Fatalf("FindByTask error handling failed: %+v", byTask)
	}
}

func TestAgentJobRepo_ClaimNext(t *testing.T) {
	db := setupAgentJobTestDB(t)
	repo := &AgentJobRepo{DB: db}

	// Insert two pending jobs
	for i := 0; i < 2; i++ {
		if _, err := repo.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		// ensure distinct createdAt ordering
		time.Sleep(10 * time.Millisecond)
	}
	claimed, err := repo.ClaimNext()
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if claimed.Status != models.AgentJobStatusClaimed {
		t.Fatalf("claimed status = %q; want claimed", claimed.Status)
	}
	if claimed.ClaimedAt == nil {
		t.Fatal("ClaimedAt nil after claim")
	}
	if claimed.Attempts != 1 {
		t.Fatalf("Attempts = %d; want 1 after claim", claimed.Attempts)
	}
	// Second call claims the other
	second, err := repo.ClaimNext()
	if err != nil {
		t.Fatalf("second ClaimNext: %v", err)
	}
	if second.ID == claimed.ID {
		t.Fatal("second claim returned same ID")
	}
	// No more pending -> sql.ErrNoRows
	if _, err := repo.ClaimNext(); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows when none pending, got %v", err)
	}
}

func TestAgentJobRepo_ResetStale(t *testing.T) {
	db := setupAgentJobTestDB(t)
	repo := &AgentJobRepo{DB: db}

	// Insert a claimed job with old claimedAt
	old := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, claimedAt, attempts) VALUES (1, 1, 'claimed', ?, 1);`, old); err != nil {
		t.Fatalf("insert stale: %v", err)
	}
	// Fresh claimed job
	if _, err := repo.Create(&models.AgentJob{Task: 2, Persona: 1, Status: models.AgentJobStatusClaimed, ClaimedAt: func() *time.Time { x := time.Now(); return &x }(), Attempts: 1}); err != nil {
		// fallback: update after create to set claimedAt to now
		t.Fatalf("create fresh claimed: %v", err)
	}
	// Reset timeout 5 minutes -> only old should reset
	n, err := repo.ResetStale(5 * time.Minute)
	if err != nil {
		t.Fatalf("ResetStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResetStale affected %d; want 1", n)
	}
	// Old job should now be pending with NULL claimedAt
	var status string
	var claimed sql.NullString
	if err := db.QueryRow(`SELECT status, claimedAt FROM agent_job WHERE task = 1;`).Scan(&status, &claimed); err != nil {
		t.Fatalf("query stale row: %v", err)
	}
	if status != models.AgentJobStatusPending {
		t.Fatalf("stale job status = %q; want pending", status)
	}
	if claimed.Valid {
		t.Fatalf("stale job claimedAt still valid: %q", claimed.String)
	}
}

func TestAgentJobRepo_UpdateStatus(t *testing.T) {
	db := setupAgentJobTestDB(t)
	repo := &AgentJobRepo{DB: db}
	created, err := repo.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ok, err := repo.UpdateStatus(created.ID, models.AgentJobStatusRunning)
	if err != nil || !ok {
		t.Fatalf("UpdateStatus: ok=%v err=%v", ok, err)
	}
	found, _ := repo.FindOne(created.ID)
	if found.Status != models.AgentJobStatusRunning {
		t.Fatalf("status after update = %q; want running", found.Status)
	}
	// Non-existent ID returns false, no error
	ok, err = repo.UpdateStatus(9999, models.AgentJobStatusFailed)
	if err != nil {
		t.Fatalf("UpdateStatus missing: %v", err)
	}
	if ok {
		t.Fatal("UpdateStatus on missing ID returned true")
	}
}

func setupAgentJobProjectTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE project (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE checklist (id INTEGER PRIMARY KEY, project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE);`); err != nil {
		t.Fatalf("create checklist: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE persona (builtin_role TEXT NOT NULL DEFAULT '',id INTEGER PRIMARY KEY AUTOINCREMENT);`); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE task (id INTEGER PRIMARY KEY, checklist INTEGER NOT NULL REFERENCES checklist(id) ON DELETE CASCADE);`); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE agent_job (
		    id         INTEGER PRIMARY KEY AUTOINCREMENT,
		    task       INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
		    persona    INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE,
		    status     TEXT NOT NULL,
		    fromStage  INTEGER,
		    toStage    INTEGER,
		    claimedAt  TEXT,
		    startedAt  TEXT,
		    finishedAt TEXT,
		    attempts   INTEGER NOT NULL DEFAULT 0,
		    error      TEXT,
		    createdAt  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		, requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');
	`); err != nil {
		t.Fatalf("create agent_job: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE agent_run (
		    id        INTEGER PRIMARY KEY AUTOINCREMENT,
		    job       INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE,
		    output    TEXT,
		    summary   TEXT,
		    exitCode  INTEGER,
		    usageJson TEXT,
		    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		, artifactJson TEXT NOT NULL DEFAULT '{}' );
	`); err != nil {
		t.Fatalf("create agent_run: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id) VALUES (1);`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id) VALUES (1), (2);`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project) VALUES (10, 1), (11, 1), (20, 2);`); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist) VALUES (1, 10), (2, 10), (3, 11), (4, 20);`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAgentJobRepo_ListActiveByProject_ReturnsActiveOnly(t *testing.T) {
	db := setupAgentJobProjectTestDB(t)
	repo := &AgentJobRepo{DB: db}
	insert := func(taskID int, status string, createdAt string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (?, 1, ?, 0, ?);`, taskID, status, createdAt); err != nil {
			t.Fatalf("insert job task %d status %s: %v", taskID, status, err)
		}
	}
	insert(1, models.AgentJobStatusPending, "2026-01-01T00:00:01Z")
	insert(2, models.AgentJobStatusClaimed, "2026-01-01T00:00:02Z")
	insert(3, models.AgentJobStatusRunning, "2026-01-01T00:00:03Z")
	insert(1, models.AgentJobStatusCompleted, "2026-01-01T00:00:04Z")
	insert(2, models.AgentJobStatusFailed, "2026-01-01T00:00:05Z")
	insert(4, models.AgentJobStatusPending, "2026-01-01T00:00:06Z")
	jobs, err := repo.ListActiveByProject(1)
	if err != nil {
		t.Fatalf("ListActiveByProject: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs; want 3 active for project 1, got %+v", len(jobs), jobs)
	}
	for _, j := range jobs {
		if j.Status == models.AgentJobStatusCompleted || j.Status == models.AgentJobStatusFailed {
			t.Fatalf("returned non-active status %q", j.Status)
		}
	}
	if jobs[0].Status != models.AgentJobStatusPending || jobs[1].Status != models.AgentJobStatusClaimed || jobs[2].Status != models.AgentJobStatusRunning {
		t.Fatalf("ordering mismatch: %+v", jobs)
	}
	if jobs[0].Task != 1 || jobs[1].Task != 2 || jobs[2].Task != 3 {
		t.Fatalf("task mapping mismatch: %+v", jobs)
	}
}

func TestAgentJobRepo_ListActiveByProject_EmptyWhenNone(t *testing.T) {
	db := setupAgentJobProjectTestDB(t)
	repo := &AgentJobRepo{DB: db}
	jobs, err := repo.ListActiveByProject(1)
	if err != nil {
		t.Fatalf("ListActiveByProject empty: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs; want 0 when none", len(jobs))
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (1, 1, 'completed', 0, '2026-01-01T00:00:01Z');`); err != nil {
		t.Fatalf("insert completed: %v", err)
	}
	jobs, err = repo.ListActiveByProject(1)
	if err != nil {
		t.Fatalf("ListActiveByProject after completed: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs; want 0 when only completed", len(jobs))
	}
}

func TestAgentJobRepo_ListActiveByProject_CompletedNotReturned(t *testing.T) {
	db := setupAgentJobProjectTestDB(t)
	repo := &AgentJobRepo{DB: db}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (1, 1, 'completed', 0, '2026-01-01T00:00:01Z');`); err != nil {
		t.Fatalf("insert completed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (2, 1, 'failed', 0, '2026-01-01T00:00:02Z');`); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (3, 1, 'pending', 0, '2026-01-01T00:00:03Z');`); err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	jobs, err := repo.ListActiveByProject(1)
	if err != nil {
		t.Fatalf("ListActiveByProject: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs; want 1 pending only, got %+v", len(jobs), jobs)
	}
	if jobs[0].Status != models.AgentJobStatusPending {
		t.Fatalf("status = %q; want pending", jobs[0].Status)
	}
}

func TestAgentJobRepo_ListFiltered_ProjectAndStatus(t *testing.T) {
	db := setupAgentJobProjectTestDB(t)
	repo := &AgentJobRepo{DB: db}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (1, 1, 'pending', 0, '2026-01-01T00:00:01Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (2, 1, 'claimed', 0, '2026-01-01T00:00:02Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (4, 1, 'pending', 0, '2026-01-01T00:00:03Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	pid := 1
	jobs, err := repo.ListFiltered(&pid, []string{"pending"})
	if err != nil {
		t.Fatalf("ListFiltered project+pending: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Task != 1 {
		t.Fatalf("got %+v; want 1 job task 1", jobs)
	}
	jobs, err = repo.ListFiltered(&pid, []string{"active"})
	if err != nil {
		t.Fatalf("ListFiltered active: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("active got %d; want 2", len(jobs))
	}
	jobs, err = repo.ListFiltered(nil, []string{"pending"})
	if err != nil {
		t.Fatalf("ListFiltered nil project pending: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("pending all projects got %d; want 2 (tasks 1 and 4)", len(jobs))
	}
}

func TestAgentJobRepo_ListActiveByProjectIDs(t *testing.T) {
	db := setupAgentJobProjectTestDB(t)
	repo := &AgentJobRepo{DB: db}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (1, 1, 'pending', 0, '2026-01-01T00:00:01Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (4, 1, 'running', 0, '2026-01-01T00:00:02Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job (task, persona, status, attempts, createdAt) VALUES (2, 1, 'completed', 0, '2026-01-01T00:00:03Z');`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	jobs, err := repo.ListActiveByProjectIDs([]int{1, 2})
	if err != nil {
		t.Fatalf("ListActiveByProjectIDs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d; want 2 active across both projects, got %+v", len(jobs), jobs)
	}
	jobs, err = repo.ListActiveByProjectIDs([]int{2})
	if err != nil {
		t.Fatalf("ListActiveByProjectIDs single: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Task != 4 {
		t.Fatalf("project 2 got %+v; want task 4", jobs)
	}
}
