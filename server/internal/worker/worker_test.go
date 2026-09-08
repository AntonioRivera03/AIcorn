package worker

import (
	"context"
	_ "modernc.org/sqlite"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func testService(t *testing.T) *services.AgentJobService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'test')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1)`,
		`INSERT INTO project(id,workflow,name) VALUES(1,1,'Test')`,
		`INSERT INTO checklist(id,project,name) VALUES(1,1,'Test')`,
		`INSERT INTO task(id,checklist,stage,type,name,priority) VALUES(1,1,1,1,'One','Medium'),(2,1,1,1,'Two','Medium')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return &services.AgentJobService{JobRepo: &repos.AgentJobRepo{DB: db}, RunRepo: &repos.AgentRunRepo{DB: db}}
}

type testEngine struct {
	calls atomic.Int32
	run   func(context.Context, harness.RunSpec) (harness.RunResult, error)
}

func (h *testEngine) Run(ctx context.Context, s harness.RunSpec) (harness.RunResult, error) {
	h.calls.Add(1)
	if h.run != nil {
		return h.run(ctx, s)
	}
	return harness.RunResult{Output: "Answer", UsageJson: "{}"}, nil
}
func enqueue(t *testing.T, s *services.AgentJobService, task int) *models.AgentJob {
	t.Helper()
	j, err := s.JobRepo.EnqueueAI(task, 0, models.AIRunRequest{Intent: "ask", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestConcurrentWorkersClaimOneAttempt(t *testing.T) {
	s := testService(t)
	j := enqueue(t, s, 1)
	h := &testEngine{}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := New(s, h).RunOnce(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.RunRepo.ListByJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h.calls.Load() != 1 || got.Status != "completed" || len(runs) != 1 {
		t.Fatalf("calls %d job %+v runs %+v", h.calls.Load(), got, runs)
	}
}

func TestRestartInterruptsWithoutReplayingAndRetainsCheckpoint(t *testing.T) {
	s := testService(t)
	j := enqueue(t, s, 1)
	enqueue(t, s, 2)
	if _, err := s.ClaimNext(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkRunning(j.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRepo.BeginAttempt(j.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRepo.Checkpoint(j.ID, "Partial answer", "{}", models.AIRunArtifacts{Workspace: "/preserved"}); err != nil {
		t.Fatal(err)
	}
	h := &testEngine{}
	w := New(s, h)
	if err := w.Start(context.Background(), time.Hour); err != nil {
		t.Fatal(err)
	}
	w.Stop()
	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.RunRepo.ListByJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "interrupted" || len(runs) != 1 || runs[0].Output != "Partial answer" || runs[0].Artifacts.Workspace != "/preserved" {
		t.Fatalf("%+v %+v", got, runs)
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.calls.Load() != 1 {
		t.Fatal("interrupted job replayed")
	}
}

func TestClaimedCancelNeverStartsEngine(t *testing.T) {
	s := testService(t)
	j := enqueue(t, s, 1)
	claimed, err := s.ClaimNext()
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.JobRepo.CancelAI(j.ID); err != nil || !ok {
		t.Fatalf("cancel: %v %v", ok, err)
	}
	h := &testEngine{}
	if _, err := New(s, h).runExplicit(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h.calls.Load() != 0 || got.Status != "canceled" {
		t.Fatalf("executed canceled job: %+v", got)
	}
}

func TestShutdownRetainsPartialOutputAndDoesNotRetry(t *testing.T) {
	s := testService(t)
	j := enqueue(t, s, 1)
	started := make(chan struct{})
	h := &testEngine{run: func(ctx context.Context, _ harness.RunSpec) (harness.RunResult, error) {
		close(started)
		<-ctx.Done()
		return harness.RunResult{Output: "Partial"}, ctx.Err()
	}}
	w := New(s, h)
	if err := w.Start(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		w.Stop()
		t.Fatal("worker did not start")
	}
	w.Stop()
	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.RunRepo.ListByJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "interrupted" || len(runs) != 1 || runs[0].Output != "Partial" || h.calls.Load() != 1 {
		t.Fatalf("%+v %+v", got, runs)
	}
}

func TestDatabaseLockIsExclusiveAndReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	release, err := AcquireDatabaseLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := AcquireDatabaseLock(path); err == nil {
		other()
		release()
		t.Fatal("second worker acquired database")
	}
	release()
	release, err = AcquireDatabaseLock(path)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
