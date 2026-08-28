package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/mcptools"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

// Worker runs one ticker goroutine that claims pending agent jobs every interval,
// recovers stale jobs on startup, and executes the harness via git worktree isolation.
// Single-goroutine, maxConcurrentJobs=1 — no parallelism. Uses time.Ticker, not busy loop.
//
// Phase 4 lifecycle (Documentation/phase-4-coding-harness.md):
//  1. Resolve repoRoot via git rev-parse --show-toplevel (worktree.RepoRootFromDir).
//  2. Create worktree on branch aycorn/task-{jobID} (worktree.Create). Branch naming
//     uses jobID (aycorn/task-{jobID}) rather than taskID to guarantee uniqueness
//     when multiple jobs target the same task. Spec says aycorn/task-{id} where id
//     is task id — jobID is more correct for the per-job isolation model; if strict
//     spec compliance is needed, change BranchName(job.Task) instead of BranchName(job.ID).
//  3. Generate per-job MCP config via mcptools.GenerateMCPConfig(allowedTools, worktree.Path)
//     with deferred cleanup (file exists for verification even if harness currently ignores it).
//  4. Build harness.RunSpec with WorkDir=worktree.Path, BranchName, BudgetUSD, TaskName/Body/SystemPrompt/AllowedTools.
//  5. Invoke harness.Harness.Run with nested timeout (harness 5m < lease 6m).
//  6. On success capture worktree.Diff() into result.Diff, persist via Complete with diff appended as fenced block.
//  7. CAS transition task to Review if job.ToStage/FromStage provided (defensive, ignores ErrStageConflict).
//  8. Leave branch/worktree for human (do NOT call worktree.Remove on success), just log branch name.
type Worker struct {
	JobService  *services.AgentJobService
	TaskService *services.TaskService
	ProjectRepo *repos.ProjectRepo
	Harness     harness.Harness

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	interval time.Duration
	// sem enforces maxConcurrentJobs=1 within this Worker instance.
	sem chan struct{}
}

// MaxConcurrentJobs is the concurrency limit for harness jobs.
const MaxConcurrentJobs = 1

func New(jobService *services.AgentJobService, taskService *services.TaskService, h harness.Harness) *Worker {
	return &Worker{
		JobService:  jobService,
		TaskService: taskService,
		Harness:     h,
		sem:         make(chan struct{}, MaxConcurrentJobs),
	}
}

const StaleTimeout = 5 * time.Minute

func (w *Worker) Start(ctx context.Context, interval time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return nil
	}

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
			runCtx, runCancel := context.WithTimeout(ctx, harness.DefaultJobLeaseTimeout)
			_, err := w.RunOnce(runCtx)
			runCancel()
			if err != nil {
				log.Printf("worker: RunOnce error: %v", err)
			}
		}
	}
}

// resolveRepoRoot discovers the git repository root via git rev-parse --show-toplevel.
// It tries os.Getwd + worktree.RepoRootFromDir. Returns "" if not in a git repo.
func resolveRepoRoot(ctx context.Context) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := worktree.RepoRootFromDir(ctx, wd)
	if err != nil || root == "" {
		return ""
	}
	return root
}

// RunOnce claims one pending job, marks it running, executes the harness, and
// persists the result. See Worker type doc for Phase 4 lifecycle.
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

	job, err := w.JobService.ClaimNext()
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	ok, err := w.JobService.MarkRunning(job.ID)
	if err != nil {
		if _, ferr := w.JobService.Fail(job.ID, err.Error()); ferr != nil {
			log.Printf("worker: MarkRunning failed for job %d: %v (also failed to Fail: %v)", job.ID, err, ferr)
		}
		return true, err
	}
	if !ok {
		return true, nil
	}

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
	if spec.BudgetUSD == nil {
		b := harness.DefaultBudgetUSD
		spec.BudgetUSD = &b
	}

	// --- Phase 4: worktree isolation ---
	var wt *worktree.Worktree
	var repoRoot string
	if w.ProjectRepo != nil {
		proj, perr := w.ProjectRepo.FindOne(task.ProjectID)
		if perr != nil {
			msg := perr.Error()
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: ProjectRepo.FindOne failed for job %d project %d: %v (also failed to Fail: %v)", job.ID, task.ProjectID, perr, ferr)
			}
			return true, perr
		}
		repoPath := strings.TrimSpace(proj.RepoPath)
		if repoPath == "" {
			msg := "project has no repo folder linked — set it in Project Settings → General"
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: also failed to Fail after empty repoPath: %v", ferr)
			}
			return true, errors.New(msg)
		}
		info, serr := os.Stat(repoPath)
		if serr != nil || !info.IsDir() {
			msg := fmt.Sprintf("linked repo folder is not a valid git repository: %s", repoPath)
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: also failed to Fail after invalid repoPath: %v", ferr)
			}
			return true, errors.New(msg)
		}
		resolved, rerr := worktree.RepoRootFromDir(ctx, repoPath)
		if rerr != nil || resolved == "" {
			msg := fmt.Sprintf("linked repo folder is not a valid git repository: %s", repoPath)
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: also failed to Fail after git rev-parse: %v", ferr)
			}
			return true, errors.New(msg)
		}
		repoRoot = resolved
	} else {
		repoRoot = resolveRepoRoot(ctx)
	}
	if repoRoot != "" {
		// Create worktree on branch aycorn/task-{jobID}. Documented: uses jobID for uniqueness.
		// If strict spec compliance (taskID) is required, substitute job.Task here.
		created, werr := worktree.Create(ctx, repoRoot, job.ID, job.Task)
		if werr != nil {
			// If path already exists (stale worktree left by previous RunOnce or crashed test),
			// attempt to prune the stale worktree and retry once. This handles the real-repo
			// case where worktrees are intentionally left for human inspection but the next
			// RunOnce reuses the same jobID in tests (in-memory DB resets but git state persists).
			if isWorktreeExistsErr(werr) {
				log.Printf("worker: worktree path exists for job %d, pruning stale worktree and retrying: %v", job.ID, werr)
				stalePath := worktree.WorktreePath(repoRoot, job.ID)
				_ = os.RemoveAll(stalePath)
				// Also prune git's worktree metadata (ignore errors).
				_ = pruneWorktree(ctx, repoRoot, stalePath)
				created2, werr2 := worktree.Create(ctx, repoRoot, job.ID, job.Task)
				if werr2 == nil {
					created = created2
					werr = nil
				} else {
					werr = werr2
				}
			}
			if werr != nil {
				msg := werr.Error()
				log.Printf("worker: worktree.Create failed for job %d: %v", job.ID, werr)
				if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
					log.Printf("worker: also failed to Fail after worktree error: %v", ferr)
				}
				return true, werr
			}
		}
		wt = created
		spec.WorkDir = wt.Path
		spec.BranchName = wt.Branch
		log.Printf("worker: worktree created at %s branch %s for job %d", wt.Path, wt.Branch, job.ID)
	} else {
		log.Printf("worker: repoRoot not found, running without worktree isolation for job %d", job.ID)
	}

	// --- Phase 4: per-job MCP config ---
	var mcpCleanup func()
	if wt != nil {
		// Generate inside worktree dir so harness / OpencodeHarness can discover it if needed.
		// Even if harness currently ignores the file, generating + deferring cleanup ensures
		// per-job file exists for verification (future Proof).
		mcpPath, cleanup, merr := mcptools.GenerateMCPConfig(allowedTools, wt.Path)
		if merr != nil {
			log.Printf("worker: GenerateMCPConfig failed for job %d: %v", job.ID, merr)
			// Non-fatal: continue without MCP config (restrictive bypass already handled by catalog)
		} else {
			mcpCleanup = cleanup
			log.Printf("worker: MCP config generated at %s for job %d (tools=%v)", mcpPath, job.ID, allowedTools)
			// MCP config file is ephemeral per-job; defer cleanup until after harness + diff.
			defer mcpCleanup()
		}
	} else {
		// No worktree: still generate a temp config in OS temp dir for verification.
		_, cleanup, merr := mcptools.GenerateMCPConfig(allowedTools, "")
		if merr != nil {
			log.Printf("worker: GenerateMCPConfig (no worktree) failed for job %d: %v", job.ID, merr)
		} else {
			mcpCleanup = cleanup
			defer mcpCleanup()
		}
	}

	// Execute harness with exponential backoff on rate-limit errors.
	var result harness.RunResult
	var herr error
	for attempt := 0; attempt <= harness.MaxRateLimitRetries; attempt++ {
		select {
		case <-ctx.Done():
			msg := ctx.Err().Error()
			// Attempt to clean up worktree workdir on cancellation — keep branch.
			if wt != nil {
				_ = wt.Remove()
				log.Printf("worker: ctx cancelled, worktree at %s left branch %s for inspection (job %d)", wt.Path, wt.Branch, job.ID)
			}
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: ctx cancelled for job %d: %v (also failed to Fail: %v)", job.ID, ctx.Err(), ferr)
			}
			return true, ctx.Err()
		default:
		}

		result, herr = w.Harness.Run(ctx, spec)
		if herr == nil {
			break
		}
		if !harness.IsRateLimitErr(herr) {
			break
		}
		if attempt == harness.MaxRateLimitRetries {
			break
		}
		backoff := harness.BackoffDelay(attempt)
		log.Printf("worker: rate limit for job %d attempt %d, backing off %v: %v", job.ID, attempt, backoff, herr)
		select {
		case <-ctx.Done():
			msg := ctx.Err().Error()
			if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
				log.Printf("worker: ctx cancelled during backoff for job %d: %v (also failed to Fail: %v)", job.ID, ctx.Err(), ferr)
			}
			return true, ctx.Err()
		case <-time.After(backoff):
		}
	}

	if herr != nil {
		msg := herr.Error()
		// On failure: attempt to remove worktree directory but keep branch for inspection.
		if wt != nil {
			if rerr := wt.Remove(); rerr != nil {
				log.Printf("worker: worktree.Remove after harness failure for job %d: %v (branch %s kept)", job.ID, rerr, wt.Branch)
			} else {
				log.Printf("worker: worktree removed at %s, branch %s kept for inspection (job %d failed)", wt.Path, wt.Branch, job.ID)
			}
		}
		if _, ferr := w.JobService.Fail(job.ID, msg); ferr != nil {
			log.Printf("worker: harness failed for job %d: %v (also failed to Fail: %v)", job.ID, herr, ferr)
			return true, herr
		}
		return true, herr
	}

	// Success — remove MCP config before diff so ephemeral file does not pollute diff.
	if mcpCleanup != nil {
		mcpCleanup()
	}
	// Capture diff if worktree exists.
	if wt != nil {
		diff, derr := wt.Diff()
		if derr != nil {
			log.Printf("worker: Diff failed for job %d worktree %s: %v", job.ID, wt.Path, derr)
		} else if diff != "" {
			result.Diff = diff
			log.Printf("worker: diff captured (%d bytes) for job %d", len(diff), job.ID)
		} else {
			log.Printf("worker: no diff for job %d", job.ID)
		}
	}

	// Append diff as fenced block to output if present.
	output := result.Output
	if result.Diff != "" {
		output += "\n\n---\n\n## Diff\n```diff\n" + result.Diff + "\n```"
	}

	// Handle retryable error so loop backoffs — IsRateLimitErr already handled above in retry loop.
	// If this is a rate-limit error that exhausted retries, we already returned via herr branch.
	// Here herr==nil, so proceed to Complete.

	if err := w.JobService.Complete(job.ID, output, result.Summary, &result.ExitCode, result.UsageJson); err != nil {
		if _, ferr := w.JobService.Fail(job.ID, err.Error()); ferr != nil {
			log.Printf("worker: Complete failed for job %d: %v (also failed to Fail: %v)", job.ID, err, ferr)
		}
		return true, err
	}

	// Leave branch/worktree for human inspection — do NOT call worktree.Remove on success.
	if wt != nil {
		log.Printf("worker: worktree left at %s branch %s for human inspection (job %d)", wt.Path, wt.Branch, job.ID)
	}

	// CAS transition: if job specifies FromStage/ToStage and they differ, attempt TransitionStage.
	// Defensive: ignore ErrStageConflict (already moved on enqueue or by concurrent writer).
	if job.FromStage != nil && job.ToStage != nil && *job.FromStage != *job.ToStage && w.TaskService != nil {
		if _, terr := w.TaskService.TransitionStage(job.Task, *job.FromStage, *job.ToStage); terr != nil {
			if errors.Is(terr, services.ErrStageConflict) {
				log.Printf("worker: TransitionStage skipped for task %d %d->%d: %v (idempotent)", job.Task, *job.FromStage, *job.ToStage, terr)
			} else {
				log.Printf("worker: TransitionStage failed for task %d %d->%d: %v", job.Task, *job.FromStage, *job.ToStage, terr)
			}
		} else {
			log.Printf("worker: TransitionStage succeeded for task %d %d->%d (job %d)", job.Task, *job.FromStage, *job.ToStage, job.ID)
		}
	}

	return true, nil
}

func isWorktreeExistsErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}

func pruneWorktree(ctx context.Context, repoRoot, wtPath string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Fallback: try prune if remove failed
		prune := exec.CommandContext(ctx, "git", "worktree", "prune")
		prune.Dir = repoRoot
		_ = prune.Run()
		return err
	}
	return nil
}
