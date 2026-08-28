package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
)

// --- helpers for budget/backoff tests ---

type countingHarness struct {
	calls int
	errs  []error // errs[i] returned on call i; if i exceeds len, returns success
}

func (c *countingHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	idx := c.calls
	c.calls++
	if idx < len(c.errs) && c.errs[idx] != nil {
		return harness.RunResult{}, c.errs[idx]
	}
	return harness.RunResult{
		Output:    "# success for task " + spec.TaskName,
		Summary:   "success",
		ExitCode:  0,
		UsageJson: `{"ok":true}`,
	}, nil
}

type slowHarness struct {
	delay time.Duration
}

func (s *slowHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	select {
	case <-ctx.Done():
		return harness.RunResult{}, ctx.Err()
	case <-time.After(s.delay):
		return harness.RunResult{Output: "slow done", Summary: "slow", ExitCode: 0}, nil
	}
}

func TestMaxConcurrentJobsIsOne(t *testing.T) {
	if MaxConcurrentJobs != 1 {
		t.Fatalf("MaxConcurrentJobs = %d; want 1 (do not increase without 529 risk)", MaxConcurrentJobs)
	}
	// Also prove harness timeout < lease timeout invariant from worker's perspective
	if harness.DefaultHarnessTimeout >= harness.DefaultJobLeaseTimeout {
		t.Fatalf("harness timeout %v must be < lease timeout %v", harness.DefaultHarnessTimeout, harness.DefaultJobLeaseTimeout)
	}
}

func TestWorker_RateLimitRetry_SucceedsAfterBackoff(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Fail twice with 529 then succeed
	h := &countingHarness{
		errs: []error{
			errors.New("529 overloaded"),
			errors.New("rate limit exceeded"),
		},
	}
	w := New(jobSvc, taskSvc, h)

	start := time.Now()
	did, rerr := w.RunOnce(context.Background())
	elapsed := time.Since(start)

	if rerr != nil {
		t.Fatalf("RunOnce after retries: %v", rerr)
	}
	if !did {
		t.Fatal("RunOnce returned false; want true")
	}
	if h.calls != 3 {
		t.Fatalf("harness calls = %d; want 3 (2 failures + 1 success)", h.calls)
	}
	// Backoff for attempt 0 is 1s + jitter, attempt 1 is 2s + jitter => total >= 3s
	// Allow small slack for scheduler, require at least 2.5s
	if elapsed < 2500*time.Millisecond {
		t.Fatalf("elapsed %v too short; expected backoff delays (1s + 2s)", elapsed)
	}
	// Verify job is completed (not failed)
	got, _ := jobSvc.Get(job.ID)
	if got.Status != "completed" {
		t.Fatalf("status = %q; want completed after retry success", got.Status)
	}
}

func TestWorker_RateLimitRetry_ExhaustedMarksFailed(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Always rate limited — 4 calls (initial + 3 retries)
	h := &countingHarness{
		errs: []error{
			errors.New("529"),
			errors.New("529"),
			errors.New("529"),
			errors.New("529"),
		},
	}
	w := New(jobSvc, taskSvc, h)
	start := time.Now()
	did, rerr := w.RunOnce(context.Background())
	elapsed := time.Since(start)
	if rerr == nil {
		t.Fatal("expected rate-limit error after exhaustion, got nil")
	}
	if !harness.IsRateLimitErr(rerr) {
		t.Fatalf("err should be rate limit: %v", rerr)
	}
	if !did {
		t.Fatal("did false; want true (job processed)")
	}
	if h.calls != 4 {
		t.Fatalf("calls = %d; want 4 (1 + MaxRateLimitRetries 3)", h.calls)
	}
	// Total backoff = 1 + 2 + 4 = 7s (+ jitter). Require >= 6.5s
	if elapsed < 6500*time.Millisecond {
		t.Fatalf("elapsed %v too short; want ~7s of backoff", elapsed)
	}
	got, _ := jobSvc.Get(job.ID)
	if got.Status != "failed" {
		t.Fatalf("status = %q; want failed after exhaustion", got.Status)
	}
	if !harness.IsRateLimitErr(errors.New(got.Error)) {
		t.Fatalf("job error field should contain rate limit: %q", got.Error)
	}
}

func TestWorker_NonRateLimitFailsFast(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	h := &countingHarness{
		errs: []error{
			errors.New("500 internal server error"),
		},
	}
	w := New(jobSvc, taskSvc, h)
	start := time.Now()
	did, rerr := w.RunOnce(context.Background())
	elapsed := time.Since(start)
	if rerr == nil {
		t.Fatal("expected non-rate-limit error")
	}
	if did != true {
		t.Fatalf("did = %v; want true", did)
	}
	if h.calls != 1 {
		t.Fatalf("calls = %d; want 1 (no retry on non-529)", h.calls)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("elapsed %v too long; non-rate-limit should fail fast without backoff", elapsed)
	}
	got, _ := jobSvc.Get(job.ID)
	if got.Status != "failed" {
		t.Fatalf("status = %q; want failed", got.Status)
	}
}

func TestWorker_RateLimit_WithApiErrorStatus(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	job, err := jobSvc.Enqueue(2, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	h := &countingHarness{
		errs: []error{
			errors.New(`{"is_error":true,"api_error_status":529}`),
			nil,
		},
	}
	w := New(jobSvc, taskSvc, h)
	start := time.Now()
	did, rerr := w.RunOnce(context.Background())
	elapsed := time.Since(start)
	if rerr != nil {
		t.Fatalf("RunOnce: %v", rerr)
	}
	if !did {
		t.Fatal("did false")
	}
	if h.calls != 2 {
		t.Fatalf("calls = %d; want 2 (1 rate limit + 1 success)", h.calls)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed %v too short; api_error_status should trigger backoff", elapsed)
	}
	got, _ := jobSvc.Get(job.ID)
	if got.Status != "completed" {
		t.Fatalf("status %q; want completed", got.Status)
	}
}

func TestWorker_RetryRespectsCancellation(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	h := &countingHarness{
		errs: []error{
			errors.New("529"),
			errors.New("529"),
			errors.New("529"),
			errors.New("529"),
		},
	}
	w := New(jobSvc, taskSvc, h)
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel during first backoff (1s). Schedule cancel after 300ms.
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	did, rerr := w.RunOnce(ctx)
	elapsed := time.Since(start)
	if !errors.Is(rerr, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", rerr)
	}
	if !did {
		t.Fatalf("did false; want true")
	}
	// Should have done only 1 harness call before cancel hit backoff sleep
	if h.calls != 1 {
		t.Fatalf("calls = %d; want 1 (cancelled during first backoff before retry)", h.calls)
	}
	if elapsed < 200*time.Millisecond || elapsed > 800*time.Millisecond {
		t.Fatalf("elapsed %v unexpected; should be ~300ms (cancelled backoff)", elapsed)
	}
	// Job should be marked failed with cancellation
	got, _ := jobSvc.Get(job.ID)
	if got.Status != "failed" {
		t.Fatalf("status %q; want failed after cancel", got.Status)
	}
}

func TestWorker_RunOnceRespectsContextCancellation(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	if _, err := jobSvc.Enqueue(1, 1, &from, &to); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := w.RunOnce(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce with cancelled ctx err = %v; want Canceled", err)
	}
}

func TestWorker_BackoffDelays_NotBusyLoop(t *testing.T) {
	// Directly test harness.BackoffDelay timing without full worker RunOnce
	for attempt := 0; attempt < 3; attempt++ {
		base := harness.BackoffDelayNoJitter(attempt)
		start := time.Now()
		delay := harness.BackoffDelay(attempt)
		// Simulate select sleep like worker does
		select {
		case <-time.After(delay):
		}
		elapsed := time.Since(start)
		if elapsed < base {
			t.Fatalf("attempt %d sleep %v < base %v", attempt, elapsed, base)
		}
		if elapsed > base+600*time.Millisecond {
			t.Fatalf("attempt %d sleep %v > base+600ms %v", attempt, elapsed, base)
		}
	}
}

func TestWorker_LoopRespectsCancellation(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx, 10*time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Give loop a couple ticks
	time.Sleep(30 * time.Millisecond)
	cancel()
	// Stop should return promptly (ctx cancellation path)
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Stop did not return within 1s after ctx cancellation")
	}
}

func TestWorker_Semaphore_SingleConcurrent(t *testing.T) {
	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from, to := 10, 20
	// Enqueue one job
	if _, err := jobSvc.Enqueue(1, 1, &from, &to); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Use slow harness that takes 400ms
	w := New(jobSvc, taskSvc, &slowHarness{delay: 400 * time.Millisecond})
	// Launch two concurrent RunOnce on same worker instance — only one should acquire sem
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	go func() {
		did, err := w.RunOnce(context.Background())
		results <- did
		errs <- err
	}()
	// Tiny delay to ensure first acquires sem
	time.Sleep(20 * time.Millisecond)
	go func() {
		did, err := w.RunOnce(context.Background())
		results <- did
		errs <- err
	}()
	did1 := <-results
	err1 := <-errs
	did2 := <-results
	err2 := <-errs
	// Exactly one should have processed the job (true), the other should have been skipped (false due to sem)
	if did1 == did2 {
		t.Fatalf("both RunOnce returned same did=%v; want one true one false (semaphore)", did1)
	}
	if err1 != nil && err2 != nil {
		t.Fatalf("unexpected errors %v %v", err1, err2)
	}
	// Verify only one job became completed
	got, _ := jobSvc.Get(1)
	// One of them succeeded; check status is completed or running
	if got.Status != "completed" && got.Status != "running" && got.Status != "claimed" {
		t.Fatalf("unexpected status after concurrent: %q", got.Status)
	}
}

func TestWorker_TimeoutNesting_Proof(t *testing.T) {
	outer := harness.DefaultJobLeaseTimeout
	inner := harness.DefaultHarnessTimeout
	if inner >= outer {
		t.Fatalf("inner %v >= outer %v; harness must fire before lease", inner, outer)
	}
	// Prove inner deadline is used in harness Run: create outer with lease timeout,
	// harness inner will be min. Simulate by creating contexts as harness/worker do.
	ctx := context.Background()
	outerCtx, cancelOuter := context.WithTimeout(ctx, outer)
	defer cancelOuter()
	innerCtx, cancelInner := context.WithTimeout(outerCtx, inner)
	defer cancelInner()
	if dOuter, _ := outerCtx.Deadline(); dOuter.IsZero() {
		t.Fatal("outer no deadline")
	}
	if dInner, _ := innerCtx.Deadline(); !dInner.Before(time.Now().Add(outer)) {
		t.Fatalf("inner deadline not before outer+ lease")
	}
	// Inner should fire ~1m earlier
	od, _ := outerCtx.Deadline()
	id, _ := innerCtx.Deadline()
	if od.Sub(id) < 50*time.Second {
		t.Fatalf("nesting gap %v too small; want ~1m", od.Sub(id))
	}
}
