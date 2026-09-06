package services

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	_ "modernc.org/sqlite"
)

func concurrentServiceDSN(t *testing.T) string {
	t.Helper()
	sanitized := strings.ReplaceAll(t.Name(), "/", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "#", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	return fmt.Sprintf("file:claim_svc_%s_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", sanitized, time.Now().UnixNano())
}

func setupConcurrentServiceDB(t *testing.T) (*sql.DB, *sql.DB, *AgentJobService, *AgentJobService) {
	t.Helper()
	dsn := concurrentServiceDSN(t)

	db1, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite db1: %v", err)
	}
	db1.SetMaxOpenConns(5)
	if _, err := db1.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		t.Fatalf("pragma WAL db1: %v", err)
	}
	t.Cleanup(func() { db1.Close() })

	db2, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite db2: %v", err)
	}
	db2.SetMaxOpenConns(5)
	if _, err := db2.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		t.Fatalf("pragma WAL db2: %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	// Schema via db1 (shared cache)
	if _, err := db1.Exec(`
		CREATE TABLE persona (
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
	if _, err := db1.Exec(`CREATE TABLE task (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db1.Exec(`
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
		);
	`); err != nil {
		t.Fatalf("create agent_job: %v", err)
	}
	if _, err := db1.Exec(`
		CREATE TABLE agent_run (
		    id        INTEGER PRIMARY KEY AUTOINCREMENT,
		    job       INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE,
		    output    TEXT,
		    summary   TEXT,
		    exitCode  INTEGER,
		    usageJson TEXT,
		    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		);
	`); err != nil {
		t.Fatalf("create agent_run: %v", err)
	}
	if _, err := db1.Exec(`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1, 'research', 'opencode', 'opencode-go/muse-spark-1.2-contributor');`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO task (id) VALUES (1), (2), (3);`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	svc1 := &AgentJobService{
		JobRepo:     &repos.AgentJobRepo{DB: db1},
		RunRepo:     &repos.AgentRunRepo{DB: db1},
		PersonaRepo: &repos.PersonaRepo{DB: db1},
		TaskRepo:    nil,
	}
	svc2 := &AgentJobService{
		JobRepo:     &repos.AgentJobRepo{DB: db2},
		RunRepo:     &repos.AgentRunRepo{DB: db2},
		PersonaRepo: &repos.PersonaRepo{DB: db2},
		TaskRepo:    nil,
	}

	return db1, db2, svc1, svc2
}

func TestAgentJobService_ClaimNext_ConcurrentExactlyOnce_OneJobTwoWorkers(t *testing.T) {
	db1, _, svc1, svc2 := setupConcurrentServiceDB(t)

	created, err := svc1.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	type result struct {
		job *models.AgentJob
		err error
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]result, 2)
	svcs := []*AgentJobService{svc1, svc2}

	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-barrier
			j, e := svcs[i].ClaimNext()
			results[i] = result{job: j, err: e}
		}()
	}
	close(barrier)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent ClaimNext service")
	}

	var successes []*models.AgentJob
	var nils int
	for idx, r := range results {
		if r.err != nil {
			t.Fatalf("worker %d err %v", idx, r.err)
		}
		if r.job != nil {
			successes = append(successes, r.job)
		} else {
			nils++
		}
	}

	if len(successes) != 1 {
		t.Fatalf("expected exactly 1 non-nil success, got %d successes, %d nils, results %+v", len(successes), nils, results)
	}
	if nils != 1 {
		t.Fatalf("expected exactly 1 nil (loser maps ErrNoRows to nil), got %d", nils)
	}

	claimed := successes[0]
	if claimed.ID != created.ID {
		t.Fatalf("claimed ID = %d; want %d", claimed.ID, created.ID)
	}
	if claimed.Status != models.AgentJobStatusClaimed {
		t.Fatalf("status = %q; want claimed", claimed.Status)
	}
	if claimed.ClaimedAt == nil {
		t.Fatal("ClaimedAt nil after concurrent claim")
	}
	if claimed.Attempts != 1 {
		t.Fatalf("Attempts = %d; want 1", claimed.Attempts)
	}

	// Verify DB state
	var pendingCnt, claimedCnt int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='pending';`).Scan(&pendingCnt); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='claimed';`).Scan(&claimedCnt); err != nil {
		t.Fatalf("count claimed: %v", err)
	}
	if pendingCnt != 0 {
		t.Fatalf("pending count = %d; want 0", pendingCnt)
	}
	if claimedCnt != 1 {
		t.Fatalf("claimed count = %d; want 1", claimedCnt)
	}
	var attempts int
	var claimedAt sql.NullString
	if err := db1.QueryRow(`SELECT attempts, claimedAt FROM agent_job WHERE id=?;`, created.ID).Scan(&attempts, &claimedAt); err != nil {
		t.Fatalf("query attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d; want 1", attempts)
	}
	if !claimedAt.Valid || claimedAt.String == "" {
		t.Fatal("claimedAt not set in DB")
	}
	// Another ClaimNext should return nil,nil
	extra, err := svc1.ClaimNext()
	if err != nil {
		t.Fatalf("extra ClaimNext err: %v", err)
	}
	if extra != nil {
		t.Fatalf("extra ClaimNext = %+v; want nil when no pending", extra)
	}
}

func TestAgentJobService_ClaimNext_ConcurrentTwoJobs(t *testing.T) {
	db1, _, svc1, svc2 := setupConcurrentServiceDB(t)

	j1, err := svc1.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue j1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	j2, err := svc1.Enqueue(2, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue j2: %v", err)
	}

	type result struct {
		job *models.AgentJob
		err error
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]result, 2)
	svcs := []*AgentJobService{svc1, svc2}

	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-barrier
			j, e := svcs[i].ClaimNext()
			results[i] = result{job: j, err: e}
		}()
	}
	close(barrier)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout two jobs concurrent")
	}

	for idx, r := range results {
		if r.err != nil {
			t.Fatalf("worker %d err %v", idx, r.err)
		}
		if r.job == nil {
			t.Fatalf("worker %d returned nil; want job", idx)
		}
		if r.job.Status != models.AgentJobStatusClaimed {
			t.Fatalf("worker %d status %q want claimed", idx, r.job.Status)
		}
		if r.job.Attempts != 1 {
			t.Fatalf("worker %d Attempts %d want 1", idx, r.job.Attempts)
		}
		if r.job.ClaimedAt == nil {
			t.Fatalf("worker %d ClaimedAt nil", idx)
		}
	}
	if results[0].job.ID == results[1].job.ID {
		t.Fatalf("both workers claimed same ID %d", results[0].job.ID)
	}
	ids := map[int]bool{results[0].job.ID: true, results[1].job.ID: true}
	if !ids[j1.ID] || !ids[j2.ID] {
		t.Fatalf("claimed IDs %v want %d and %d", ids, j1.ID, j2.ID)
	}

	var pendingCnt, claimedCnt int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='pending';`).Scan(&pendingCnt); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='claimed';`).Scan(&claimedCnt); err != nil {
		t.Fatalf("count claimed: %v", err)
	}
	if pendingCnt != 0 {
		t.Fatalf("pending %d want 0", pendingCnt)
	}
	if claimedCnt != 2 {
		t.Fatalf("claimed %d want 2", claimedCnt)
	}
	for _, id := range []int{j1.ID, j2.ID} {
		var attempts int
		var claimedAt sql.NullString
		if err := db1.QueryRow(`SELECT attempts, claimedAt FROM agent_job WHERE id=?;`, id).Scan(&attempts, &claimedAt); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		if attempts != 1 {
			t.Fatalf("job %d attempts %d want 1", id, attempts)
		}
		if !claimedAt.Valid || claimedAt.String == "" {
			t.Fatalf("job %d claimedAt not set", id)
		}
	}
	// No more pending -> service maps ErrNoRows to nil
	extra, err := svc1.ClaimNext()
	if err != nil {
		t.Fatalf("extra ClaimNext err %v", err)
	}
	if extra != nil {
		t.Fatalf("extra ClaimNext %v want nil", extra)
	}
}
