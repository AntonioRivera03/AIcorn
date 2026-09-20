package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	_ "modernc.org/sqlite"
)

func setupAgentJobServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE persona (builtin_role TEXT NOT NULL DEFAULT '',
		    id            INTEGER PRIMARY KEY AUTOINCREMENT,
		    name          TEXT NOT NULL DEFAULT '',
		    system_prompt TEXT NOT NULL DEFAULT '',
		    harness       TEXT NOT NULL DEFAULT 'opencode',
		    model         TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor',
		    agent         TEXT NOT NULL DEFAULT '',
		    allowed_tools TEXT NOT NULL DEFAULT '[]',
		    timeCreated   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		    timeModified  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		);
	`); err != nil {
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
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1, 'research', 'opencode', 'opencode-go/muse-spark-1.2-contributor');`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id) VALUES (1), (2), (3);`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newAgentJobService(t *testing.T) (*AgentJobService, *sql.DB) {
	t.Helper()
	db := setupAgentJobServiceTestDB(t)
	svc := &AgentJobService{
		JobRepo:     &repos.AgentJobRepo{DB: db},
		RunRepo:     &repos.AgentRunRepo{DB: db},
		PersonaRepo: &repos.PersonaRepo{DB: db},
		TaskRepo:    nil,
	}
	return svc, db
}

func TestAgentJobService_EnqueueCreatesPending(t *testing.T) {
	svc, _ := newAgentJobService(t)
	from := 10
	to := 20
	job, err := svc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.Status != models.AgentJobStatusPending {
		t.Fatalf("status = %q; want pending", job.Status)
	}
	if job.Attempts != 0 {
		t.Fatalf("Attempts = %d; want 0", job.Attempts)
	}
	if job.FromStage == nil || *job.FromStage != from {
		t.Fatalf("FromStage = %v; want %d", job.FromStage, from)
	}
	if job.ToStage == nil || *job.ToStage != to {
		t.Fatalf("ToStage = %v; want %d", job.ToStage, to)
	}
	if job.CreatedAt == nil {
		t.Fatal("CreatedAt nil")
	}
	found, err := svc.Get(job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found.ID != job.ID || found.Status != models.AgentJobStatusPending {
		t.Fatalf("Get mismatch: %+v", found)
	}
}

func TestAgentJobService_EnqueueInvalidPersona(t *testing.T) {
	svc, _ := newAgentJobService(t)
	_, err := svc.Enqueue(1, 999, nil, nil)
	if err != ErrInvalidPersona {
		t.Fatalf("expected ErrInvalidPersona, got %v", err)
	}
}

func TestAgentJobService_ClaimNext_MovesToClaimedAndExclusive(t *testing.T) {
	svc, _ := newAgentJobService(t)
	j1, err := svc.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue j1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	j2, err := svc.Enqueue(2, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue j2: %v", err)
	}
	claimed, err := svc.ClaimNext()
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNext returned nil; want job")
	}
	if claimed.ID != j1.ID {
		t.Fatalf("claimed ID = %d; want oldest %d", claimed.ID, j1.ID)
	}
	if claimed.Status != models.AgentJobStatusClaimed {
		t.Fatalf("status = %q; want claimed", claimed.Status)
	}
	if claimed.ClaimedAt == nil {
		t.Fatal("ClaimedAt nil after claim")
	}
	if claimed.Attempts != 1 {
		t.Fatalf("Attempts = %d; want 1", claimed.Attempts)
	}
	second, err := svc.ClaimNext()
	if err != nil {
		t.Fatalf("second ClaimNext: %v", err)
	}
	if second == nil || second.ID != j2.ID {
		t.Fatalf("second claim ID = %v; want %d", second, j2.ID)
	}
	third, err := svc.ClaimNext()
	if err != nil {
		t.Fatalf("third ClaimNext: %v", err)
	}
	if third != nil {
		t.Fatalf("third ClaimNext = %+v; want nil (no pending)", third)
	}
}

func TestAgentJobService_ClaimNext_NilWhenEmpty(t *testing.T) {
	svc, _ := newAgentJobService(t)
	job, err := svc.ClaimNext()
	if err != nil {
		t.Fatalf("ClaimNext on empty: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil when no pending, got %+v", job)
	}
}

func TestAgentJobService_ResetStale_MovesOldClaimedBack(t *testing.T) {
	svc, db := newAgentJobService(t)
	job, err := svc.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := svc.ClaimNext()
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext: err=%v job=%v", err, claimed)
	}
	if claimed.ID != job.ID {
		t.Fatalf("claimed mismatch")
	}
	old := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE agent_job SET claimedAt = ? WHERE id = ?;`, old, job.ID); err != nil {
		t.Fatalf("backdate claimedAt: %v", err)
	}
	n, err := svc.ResetStale(5 * time.Minute)
	if err != nil {
		t.Fatalf("ResetStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResetStale count = %d; want 1", n)
	}
	found, _ := svc.Get(job.ID)
	if found.Status != models.AgentJobStatusPending {
		t.Fatalf("after reset status = %q; want pending", found.Status)
	}
	if found.ClaimedAt != nil {
		t.Fatalf("ClaimedAt not nil after reset: %v", found.ClaimedAt)
	}
	again, err := svc.ClaimNext()
	if err != nil || again == nil {
		t.Fatalf("ClaimNext after reset: err=%v job=%v", err, again)
	}
	if again.ID != job.ID {
		t.Fatalf("re-claimed ID = %d; want %d", again.ID, job.ID)
	}
}

func TestAgentJobService_Complete_WritesRunAndMarksFinished(t *testing.T) {
	svc, _ := newAgentJobService(t)
	job, _ := svc.Enqueue(1, 1, nil, nil)
	claimed, err := svc.ClaimNext()
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext: %v %v", err, claimed)
	}
	exit := 0
	usage := `{"prompt_tokens":10}`
	if err := svc.Complete(job.ID, "# output", "did work", &exit, usage); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	found, _ := svc.Get(job.ID)
	if found.Status != models.AgentJobStatusCompleted {
		t.Fatalf("status after Complete = %q; want completed", found.Status)
	}
	if found.FinishedAt == nil {
		t.Fatal("FinishedAt nil after Complete")
	}
	runs, err := svc.RunRepo.ListByJob(job.ID)
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs count = %d; want 1", len(runs))
	}
	if runs[0].Output != "# output" {
		t.Fatalf("Output = %q; want # output", runs[0].Output)
	}
	if runs[0].Summary != "did work" {
		t.Fatalf("Summary = %q", runs[0].Summary)
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 0 {
		t.Fatalf("ExitCode = %v; want 0", runs[0].ExitCode)
	}
	if runs[0].UsageJson != usage {
		t.Fatalf("UsageJson = %q; want %q", runs[0].UsageJson, usage)
	}
}

func TestAgentJobService_Complete_FailedOnNonZeroExit(t *testing.T) {
	svc, _ := newAgentJobService(t)
	job, _ := svc.Enqueue(1, 1, nil, nil)
	if _, err := svc.ClaimNext(); err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	exit := 1
	if err := svc.Complete(job.ID, "failed out", "error", &exit, `{"x":1}`); err != nil {
		t.Fatalf("Complete failed exit: %v", err)
	}
	found, _ := svc.Get(job.ID)
	if found.Status != models.AgentJobStatusFailed {
		t.Fatalf("status = %q; want failed", found.Status)
	}
}

func TestAgentJobService_Complete_RejectsInvalidStatus(t *testing.T) {
	svc, _ := newAgentJobService(t)
	job, _ := svc.Enqueue(1, 1, nil, nil)
	exit := 0
	err := svc.Complete(job.ID, "out", "sum", &exit, "")
	if err != ErrInvalidJobStatus {
		t.Fatalf("expected ErrInvalidJobStatus for pending->complete, got %v", err)
	}
	// After claiming it should succeed
	if _, err := svc.ClaimNext(); err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if err := svc.Complete(job.ID, "out", "sum", &exit, ""); err != nil {
		t.Fatalf("Complete after claim: %v", err)
	}
	// Second complete should fail — already terminal
	err = svc.Complete(job.ID, "out2", "sum2", &exit, "")
	if err != ErrInvalidJobStatus {
		t.Fatalf("expected ErrInvalidJobStatus for completed->complete, got %v", err)
	}
}

func TestAgentJobService_DuplicateClaimRace_ReturnsNil(t *testing.T) {
	svc, _ := newAgentJobService(t)
	if _, err := svc.Enqueue(1, 1, nil, nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	first, err := svc.ClaimNext()
	if err != nil || first == nil {
		t.Fatalf("first ClaimNext: %v %v", err, first)
	}
	second, err := svc.ClaimNext()
	if err != nil {
		t.Fatalf("second ClaimNext err: %v", err)
	}
	if second != nil {
		t.Fatalf("second ClaimNext = %+v; want nil (only one worker owns job)", second)
	}
	// Verify DB state: exactly one claimed
	claimedJobs, err := svc.ListByStatus(models.AgentJobStatusClaimed)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(claimedJobs) != 1 || claimedJobs[0].ID != first.ID {
		t.Fatalf("claimed jobs = %v; want single %d", claimedJobs, first.ID)
	}
}

func TestAgentJobService_MarkRunning(t *testing.T) {
	svc, _ := newAgentJobService(t)
	job, _ := svc.Enqueue(1, 1, nil, nil)
	if _, err := svc.MarkRunning(job.ID); err != ErrInvalidJobStatus {
		t.Fatalf("MarkRunning on pending should be ErrInvalidJobStatus, got %v", err)
	}
	if _, err := svc.ClaimNext(); err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	ok, err := svc.MarkRunning(job.ID)
	if err != nil || !ok {
		t.Fatalf("MarkRunning after claim: ok=%v err=%v", ok, err)
	}
	found, _ := svc.Get(job.ID)
	if found.Status != models.AgentJobStatusRunning {
		t.Fatalf("status = %q; want running", found.Status)
	}
	if found.StartedAt == nil {
		t.Fatal("StartedAt nil after MarkRunning")
	}
}
