package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

// Worker runs one ticker goroutine that claims pending agent jobs every interval,
// recovers stale jobs on startup, and executes the harness shim.
// Single-goroutine, maxConcurrentJobs=1 — no parallelism. Uses time.Ticker, not busy loop.
//
// Design decision (Phase 3 minimal): worker completes the job and persists
// shim output to agent_run, but does NOT transition the ticket stage. The task
// was already transitioned to its working stage when the job was enqueued
// (TransitionStage/BulkUpdate enqueue side-effect). Advancing to Review/Done is
// left to the user or a future phase that decides "which Review stage".
// This is documented here to avoid the ambiguity noted in the task spec.
type Worker struct {
	JobService  *services.AgentJobService
	TaskService *services.TaskService
	Harness     harness.Harness

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	interval time.Duration
}

// New creates a Worker. All pointer fields must be non-nil before Start, though
// TaskService may be nil (task fetch will fall back to JobService.TaskRepo).
func New(jobService *services.AgentJobService, taskService *services.TaskService, h harness.Harness) *Worker {
	return &Worker{
		JobService:  jobService,
		TaskService: taskService,
		Harness:     h,
	}
}

// StaleTimeout is how long a claimed job may stay claimed before ResetStale
// returns it to pending. Matches the spec: 5 minutes.
const StaleTimeout = 5 * time.Minute

// Start launches the single ticker goroutine. It first recovers stale jobs
// (claimed > StaleTimeout) so a previous crash does not orphan work. The
// goroutine runs until ctx is cancelled or Stop is called. interval is typically
// 10 * time.Second per spec. Calling Start twice is a no-op (second call returns nil).
func (w *Worker) Start(ctx context.Context, interval time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return nil // already started
	}

	// Recover stale on startup. Best-effort — log and continue.
	if w.JobService != nil {
		if n, err := w.JobService.ResetStale(StaleTimeout); err != nil {
			log.Printf("worker: ResetStale failed: %v", err)
		} else if n > 0 {
			log.Printf("worker: recovered %d stale job(s) to pending", n)
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.interval = interval

	go w.loop(cctx, interval)

	return nil
}

// Stop cancels the ticker goroutine and waits for it to exit. Safe to call
// even if Start was never called.
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
			runCtx, runCancel := context.WithTimeout(ctx, 30*time.Second)
			_, err := w.RunOnce(runCtx)
			runCancel()
			if err != nil {
				log.Printf("worker: RunOnce error: %v", err)
			}
		}
	}
}

// RunOnce claims one pending job, marks it running, executes the harness, and
// persists the result. Returns (true, nil) when a job was processed, (false, nil)
// when no pending job existed, or (false/true, err) on failure.
// It is safe to call concurrently but only one ticker goroutine does so in prod,
// ensuring maxConcurrentJobs=1.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	if w.JobService == nil || w.Harness == nil {
		return false, nil
	}

	job, err := w.JobService.ClaimNext()
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	// Claim succeeded → transition claimed → running. If this fails the job
	// is already claimed; we fail it so it does not stay stuck.
	ok, err := w.JobService.MarkRunning(job.ID)
	if err != nil {
		// Attempt to fail the job so it becomes retryable/visible
		if _, ferr := w.JobService.Fail(job.ID, err.Error()); ferr != nil {
			log.Printf("worker: MarkRunning failed for job %d: %v (also failed to Fail: %v)", job.ID, err, ferr)
		}
		return true, err
	}
	if !ok {
		// CAS failed — job left claimed state concurrently (should not happen
		// with single worker, but handle defensively).
		return true, nil
	}

	// Build RunSpec from task + persona rows.
	spec := harness.RunSpec{
		JobID:     job.ID,
		TaskID:    job.Task,
		PersonaID: job.Persona,
	}

	var taskName, taskBody string
	var personaPrompt string
	var allowedTools []string

	task, terr := w.JobService.LoadTaskForRun(job.Task)
	if terr != nil {
		msg := terr.Error()
		if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
			log.Printf("worker: LoadTaskForRun failed for job %d task %d: %v (also failed to Fail: %v)", job.ID, job.Task, terr, ferr)
		}
		return true, terr
	}
	taskName = task.Name
	taskBody = task.Body

	if p, perr := w.JobService.LoadPersonaForRun(job.Persona); perr == nil && p != nil {
		personaPrompt = p.SystemPrompt
		allowedTools = p.AllowedTools
	} else if perr != nil {
		msg := perr.Error()
		if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
			log.Printf("worker: LoadPersonaForRun failed for job %d persona %d: %v (also failed to Fail: %v)", job.ID, job.Persona, perr, ferr)
		}
		return true, perr
	}

	spec.TaskName = taskName
	spec.TaskBody = taskBody
	spec.SystemPrompt = personaPrompt
	spec.AllowedTools = allowedTools

	result, herr := w.Harness.Run(ctx, spec)
	if herr != nil {
		// Harness error → mark job failed (creates terminal state + records error)
		msg := herr.Error()
		if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
			log.Printf("worker: harness failed for job %d: %v (also failed to Fail: %v)", job.ID, herr, ferr)
			return true, herr
		}
		return true, herr
	}

	// Success — persist markdown to agent_run and mark completed (or failed if non-zero exit).
	if err := w.JobService.Complete(job.ID, result.Output, result.Summary, &result.ExitCode, result.UsageJson); err != nil {
		// Complete enforces claimed/running → check; if it fails try Fail as fallback
		if _, ferr := w.JobService.Fail(job.ID, err.Error()); ferr != nil {
			log.Printf("worker: Complete failed for job %d: %v (also failed to Fail: %v)", job.ID, err, ferr)
		}
		return true, err
	}

	// Phase 3 completes job and writes run, does NOT auto-transition — human
	// moves Review→Coding is the gate per ai-architecture.md §2. The task
	// remains in its enqueued stage (job.ToStage).
	return true, nil
}
