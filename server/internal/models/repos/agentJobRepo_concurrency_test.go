package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	_ "modernc.org/sqlite"
)

func concurrentAgentJobDSN(t *testing.T) string {
	t.Helper()
	sanitized := strings.ReplaceAll(t.Name(), "/", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "#", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	return fmt.Sprintf("file:claim_%s_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", sanitized, time.Now().UnixNano())
}

func setupConcurrentAgentJobDB(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()
	dsn := concurrentAgentJobDSN(t)

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

	// Create schema once via db1 (shared cache so db2 sees it)
	if _, err := db1.Exec(`CREATE TABLE persona (id INTEGER PRIMARY KEY AUTOINCREMENT);`); err != nil {
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
		, requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');
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
		, artifactJson TEXT NOT NULL DEFAULT '{}' );
	`); err != nil {
		t.Fatalf("create agent_run: %v", err)
	}
	if _, err := db1.Exec(`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO persona (id) VALUES (1);`); err != nil {
		t.Fatalf("seed persona: %v", err)
	}
	if _, err := db1.Exec(`INSERT INTO task (id) VALUES (1), (2), (3), (4), (5);`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	return db1, db2
}

func TestAgentJobRepo_ClaimNext_ConcurrentExactlyOnce_OneJobTwoWorkers(t *testing.T) {
	db1, db2 := setupConcurrentAgentJobDB(t)
	repo1 := &AgentJobRepo{DB: db1}
	repo2 := &AgentJobRepo{DB: db2}

	created, err := repo1.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	type result struct {
		job *models.AgentJob
		err error
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]result, 2)
	repos := []*AgentJobRepo{repo1, repo2}

	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-barrier
			j, e := repos[i].ClaimNext()
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
		t.Fatal("timeout waiting for concurrent ClaimNext")
	}

	var successes []*models.AgentJob
	var noRows int
	for idx, r := range results {
		if r.err == nil && r.job != nil {
			successes = append(successes, r.job)
		} else if errors.Is(r.err, sql.ErrNoRows) {
			noRows++
		} else {
			t.Fatalf("worker %d unexpected result: job=%v err=%v", idx, r.job, r.err)
		}
	}

	if len(successes) != 1 {
		t.Fatalf("expected exactly 1 success, got %d successes, %d ErrNoRows, results=%+v", len(successes), noRows, results)
	}
	if noRows != 1 {
		t.Fatalf("expected exactly 1 ErrNoRows, got %d", noRows)
	}

	claimed := successes[0]
	if claimed.ID != created.ID {
		t.Fatalf("claimed ID = %d; want %d", claimed.ID, created.ID)
	}
	if claimed.Status != models.AgentJobStatusClaimed {
		t.Fatalf("claimed status = %q; want claimed", claimed.Status)
	}
	if claimed.ClaimedAt == nil {
		t.Fatal("ClaimedAt nil after concurrent claim")
	}
	if claimed.Attempts != 1 {
		t.Fatalf("Attempts = %d; want 1", claimed.Attempts)
	}

	// Verify DB state: pending decremented, claimed count 1, attempts only once.
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
		t.Fatalf("attempts = %d; want 1 (incremented only once)", attempts)
	}
	if !claimedAt.Valid || claimedAt.String == "" {
		t.Fatal("claimedAt not set in DB")
	}
	// Ensure no duplicate claim: the pending count should be 0 and second ClaimNext still ErrNoRows
	if _, err := repo1.ClaimNext(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("extra ClaimNext after concurrent race expected ErrNoRows, got %v", err)
	}
}

func TestAgentJobRepo_ClaimNext_ConcurrentExactlyOnce_TwoJobsTwoWorkers(t *testing.T) {
	db1, db2 := setupConcurrentAgentJobDB(t)
	repo1 := &AgentJobRepo{DB: db1}
	repo2 := &AgentJobRepo{DB: db2}

	j1, err := repo1.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create j1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	j2, err := repo1.Create(&models.AgentJob{Task: 2, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create j2: %v", err)
	}

	type result struct {
		job *models.AgentJob
		err error
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]result, 2)
	repos := []*AgentJobRepo{repo1, repo2}

	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-barrier
			j, e := repos[i].ClaimNext()
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
		t.Fatal("timeout waiting for concurrent ClaimNext two jobs")
	}

	for idx, r := range results {
		if r.err != nil {
			t.Fatalf("worker %d failed: %v", idx, r.err)
		}
		if r.job == nil {
			t.Fatalf("worker %d returned nil job", idx)
		}
		if r.job.Status != models.AgentJobStatusClaimed {
			t.Fatalf("worker %d status = %q; want claimed", idx, r.job.Status)
		}
		if r.job.ClaimedAt == nil {
			t.Fatalf("worker %d ClaimedAt nil", idx)
		}
		if r.job.Attempts != 1 {
			t.Fatalf("worker %d Attempts = %d; want 1", idx, r.job.Attempts)
		}
	}

	if results[0].job.ID == results[1].job.ID {
		t.Fatalf("both workers claimed same ID %d; want distinct", results[0].job.ID)
	}
	ids := map[int]bool{results[0].job.ID: true, results[1].job.ID: true}
	if !ids[j1.ID] || !ids[j2.ID] {
		t.Fatalf("claimed IDs %v; want %d and %d", ids, j1.ID, j2.ID)
	}

	var pendingCnt, claimedCnt int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='pending';`).Scan(&pendingCnt); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='claimed';`).Scan(&claimedCnt); err != nil {
		t.Fatalf("count claimed: %v", err)
	}
	if pendingCnt != 0 {
		t.Fatalf("pending count = %d; want 0 after claiming both", pendingCnt)
	}
	if claimedCnt != 2 {
		t.Fatalf("claimed count = %d; want 2", claimedCnt)
	}
	// Verify each job attempts incremented exactly once
	for _, id := range []int{j1.ID, j2.ID} {
		var attempts int
		var claimedAt sql.NullString
		if err := db1.QueryRow(`SELECT attempts, claimedAt FROM agent_job WHERE id=?;`, id).Scan(&attempts, &claimedAt); err != nil {
			t.Fatalf("query attempts id %d: %v", id, err)
		}
		if attempts != 1 {
			t.Fatalf("job %d attempts = %d; want 1", id, attempts)
		}
		if !claimedAt.Valid || claimedAt.String == "" {
			t.Fatalf("job %d claimedAt not set", id)
		}
	}
	// No more pending
	if _, err := repo1.ClaimNext(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows when no pending left, got %v", err)
	}
}

func TestAgentJobRepo_ClaimNext_ConcurrentManyWorkers_OneJob(t *testing.T) {
	db1, db2 := setupConcurrentAgentJobDB(t)
	// Need 5 distinct handles sharing same DSN for a stronger race; open 3 more.
	// Reuse dsn by opening additional connections from the same shared memory.
	// We already have 2; open 3 more.
	dsnTag := ""
	// Derive DSN from open db: we know naming from helper but we need extra conns.
	// Simpler: reopen using the same helper DSN trick by peeking at db1 stats is not needed.
	// Instead just use db1 with higher concurrency but also add extra DBs via new helper call is isolated.
	// So we create additional DBs by querying the DSN again using a fresh unique-ish but actually same file?
	// To avoid complexity, just use goroutines sharing db1/db2 alternately with 5 workers – still proves barrier.
	created, err := (&AgentJobRepo{DB: db1}).Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = dsnTag

	const workers = 5
	type result struct {
		job *models.AgentJob
		err error
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]result, workers)
	// Alternate repos for workers to still exercise cross-connection
	repos := make([]*AgentJobRepo, workers)
	for i := 0; i < workers; i++ {
		if i%2 == 0 {
			repos[i] = &AgentJobRepo{DB: db1}
		} else {
			repos[i] = &AgentJobRepo{DB: db2}
		}
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-barrier
			j, e := repos[i].ClaimNext()
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
		t.Fatal("timeout waiting for many-worker ClaimNext")
	}

	var success int
	var noRows int
	for idx, r := range results {
		if r.err == nil && r.job != nil {
			success++
			if r.job.ID != created.ID {
				t.Fatalf("worker %d claimed wrong ID %d; want %d", idx, r.job.ID, created.ID)
			}
			if r.job.Attempts != 1 {
				t.Fatalf("worker %d Attempts %d; want 1", idx, r.job.Attempts)
			}
		} else if errors.Is(r.err, sql.ErrNoRows) {
			noRows++
		} else {
			t.Fatalf("worker %d unexpected err %v job %v", idx, r.err, r.job)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 success among %d workers, got %d successes, %d ErrNoRows results=%+v", workers, success, noRows, results)
	}
	if noRows != workers-1 {
		t.Fatalf("expected %d ErrNoRows, got %d", workers-1, noRows)
	}

	var pendingCnt, claimedCnt int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='pending';`).Scan(&pendingCnt); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if err := db1.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE status='claimed';`).Scan(&claimedCnt); err != nil {
		t.Fatalf("count claimed: %v", err)
	}
	if pendingCnt != 0 || claimedCnt != 1 {
		t.Fatalf("counts pending=%d claimed=%d; want 0,1", pendingCnt, claimedCnt)
	}
	var attempts int
	if err := db1.QueryRow(`SELECT attempts FROM agent_job WHERE id=?;`, created.ID).Scan(&attempts); err != nil {
		t.Fatalf("query attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d; want 1", attempts)
	}
}
