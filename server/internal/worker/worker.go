package worker

import (
	"context"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"log"
	"sync"
	"time"
)

// Worker executes requests sequentially and interrupts uncertain runs on startup.
// Only opt-in Conductor requests participate in automatic workflow transitions.
type Worker struct {
	Conductor  *services.ConductorService
	JobService *services.AgentJobService
	Harness    harness.Harness

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	interval time.Duration
	// sem enforces maxConcurrentJobs=1 within this Worker instance.
	sem chan struct{}
}

// MaxConcurrentJobs is the concurrency limit for harness jobs.
const MaxConcurrentJobs = 1

func New(jobService *services.AgentJobService, h harness.Harness) *Worker {
	return &Worker{
		JobService: jobService,
		Harness:    h,
		sem:        make(chan struct{}, MaxConcurrentJobs),
	}
}

func (w *Worker) Start(ctx context.Context, interval time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return nil
	}

	if w.JobService != nil {
		if err := w.JobService.JobRepo.InterruptInFlight(); err != nil {
			return err
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.interval = interval
	if w.sem == nil {
		w.sem = make(chan struct{}, MaxConcurrentJobs)
	}

	go w.loop(cctx, interval)

	return nil
}

func (w *Worker) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
	w.mu.Lock()
	w.cancel = nil
	w.done = nil
	w.mu.Unlock()
}

func (w *Worker) loop(ctx context.Context, interval time.Duration) {
	defer close(w.done)

	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCtx, runCancel := context.WithCancel(ctx)

			_, err := w.RunOnce(runCtx)
			runCancel()
			if err != nil {
				log.Printf("worker: RunOnce error: %v", err)
			}
		}
	}
}

// RunOnce claims one pending job, marks it running, executes the harness, and
// persists the result. Jobs without an explicit snapshot are never executed.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if w.JobService == nil || w.Harness == nil {
		return false, nil
	}

	if w.sem != nil {
		select {
		case w.sem <- struct{}{}:
			defer func() { <-w.sem }()
		default:
			return false, nil
		}
	}

	if w.Conductor != nil {
		if err := w.Conductor.Tick(ctx); err != nil {
			return false, err
		}
	}
	var job *models.AgentJob
	var err error
	if w.Conductor != nil {
		job, err = w.Conductor.Repo.ClaimNext()
	} else {
		job, err = w.JobService.ClaimNext()
	}
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	return w.runExplicit(ctx, job)
}
