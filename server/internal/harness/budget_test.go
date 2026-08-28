package harness

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultBudgetUSD(t *testing.T) {
	if DefaultBudgetUSD != 5.0 {
		t.Fatalf("DefaultBudgetUSD = %v; want 5.0", DefaultBudgetUSD)
	}
}

func TestDefaultTimeouts(t *testing.T) {
	if DefaultHarnessTimeout != 5*time.Minute {
		t.Fatalf("DefaultHarnessTimeout = %v; want 5m", DefaultHarnessTimeout)
	}
	if DefaultJobLeaseTimeout != 6*time.Minute {
		t.Fatalf("DefaultJobLeaseTimeout = %v; want 6m", DefaultJobLeaseTimeout)
	}
	if MaxBackoff != 30*time.Second {
		t.Fatalf("MaxBackoff = %v; want 30s", MaxBackoff)
	}
	if MaxRateLimitRetries != 3 {
		t.Fatalf("MaxRateLimitRetries = %d; want 3", MaxRateLimitRetries)
	}
}

func TestTimeoutNesting_HarnessFiresBeforeLease(t *testing.T) {
	if DefaultHarnessTimeout >= DefaultJobLeaseTimeout {
		t.Fatalf("harness timeout %v must be < job lease timeout %v (nested invariant)", DefaultHarnessTimeout, DefaultJobLeaseTimeout)
	}
	// Prove via actual context nesting: outer 6m, inner 5m -> inner deadline is earlier.
	outer, cancelOuter := context.WithTimeout(context.Background(), DefaultJobLeaseTimeout)
	defer cancelOuter()
	inner, cancelInner := context.WithTimeout(outer, DefaultHarnessTimeout)
	defer cancelInner()
	outerDeadline, okOuter := outer.Deadline()
	innerDeadline, okInner := inner.Deadline()
	if !okOuter || !okInner {
		t.Fatal("expected deadlines for both contexts")
	}
	if !innerDeadline.Before(outerDeadline) {
		t.Fatalf("inner deadline %v should be before outer %v", innerDeadline, outerDeadline)
	}
	// Difference should be ~1 minute
	diff := outerDeadline.Sub(innerDeadline)
	if diff < 50*time.Second || diff > 70*time.Second {
		t.Fatalf("deadline diff = %v; want ~1m (5m vs 6m)", diff)
	}
}

func TestEffectiveBudget(t *testing.T) {
	// Nil → default
	spec := RunSpec{}
	if got := EffectiveBudget(spec); got != DefaultBudgetUSD {
		t.Fatalf("EffectiveBudget(nil) = %v; want %v", got, DefaultBudgetUSD)
	}
	// Zero/negative → default
	z := 0.0
	spec.BudgetUSD = &z
	if got := EffectiveBudget(spec); got != DefaultBudgetUSD {
		t.Fatalf("EffectiveBudget(0) = %v; want default", got)
	}
	neg := -1.0
	spec.BudgetUSD = &neg
	if got := EffectiveBudget(spec); got != DefaultBudgetUSD {
		t.Fatalf("EffectiveBudget(-1) = %v; want default", got)
	}
	// Positive → used
	v := 12.5
	spec.BudgetUSD = &v
	if got := EffectiveBudget(spec); got != 12.5 {
		t.Fatalf("EffectiveBudget(12.5) = %v; want 12.5", got)
	}
}

func TestWithBudget(t *testing.T) {
	ctx := context.Background()
	bCtx := WithBudget(ctx, 7.5)
	got, ok := BudgetFromContext(bCtx)
	if !ok || got != 7.5 {
		t.Fatalf("BudgetFromContext = %v %v; want 7.5 true", got, ok)
	}
	// Original ctx has no budget
	if _, ok := BudgetFromContext(ctx); ok {
		t.Fatal("original ctx should not have budget")
	}
}

func TestIsRateLimitErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"529 plain", errors.New("529 Too Many Requests"), true},
		{"529 json", errors.New(`{"code":529,"message":"overloaded"}`), true},
		{"rate limit lower", errors.New("rate limit exceeded"), true},
		{"rate limit upper", errors.New("Rate Limit hit"), true},
		{"rate_limit underscore", errors.New("rate_limit error"), true},
		{"RATE_LIMIT upper underscore", errors.New("RATE_LIMIT"), true},
		{"api_error_status", errors.New(`{"api_error_status": 529}`), true},
		{"api_error_status case", errors.New("API_ERROR_STATUS: 529"), true},
		{"not rate limit", errors.New("some other error"), false},
		{"500 not rate limit", errors.New("500 internal server error"), false},
		{"empty", errors.New(""), false},
		{"contains 5290 still true (contains 529)", errors.New("error 5290"), true},
	}
	for _, tc := range cases {
		if got := IsRateLimitErr(tc.err); got != tc.want {
			t.Errorf("%s: IsRateLimitErr(%q) = %v; want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

func TestBackoffDelay_Intervals(t *testing.T) {
	// Exact base without jitter
	if got := BackoffDelayNoJitter(0); got != 1*time.Second {
		t.Fatalf("attempt 0 no jitter = %v; want 1s", got)
	}
	if got := BackoffDelayNoJitter(1); got != 2*time.Second {
		t.Fatalf("attempt 1 no jitter = %v; want 2s", got)
	}
	if got := BackoffDelayNoJitter(2); got != 4*time.Second {
		t.Fatalf("attempt 2 no jitter = %v; want 4s", got)
	}
	if got := BackoffDelayNoJitter(3); got != 8*time.Second {
		t.Fatalf("attempt 3 no jitter = %v; want 8s", got)
	}
	if got := BackoffDelayNoJitter(4); got != 16*time.Second {
		t.Fatalf("attempt 4 no jitter = %v; want 16s", got)
	}
	// Capped at 30s
	if got := BackoffDelayNoJitter(5); got != 30*time.Second {
		t.Fatalf("attempt 5 no jitter = %v; want 30s cap", got)
	}
	if got := BackoffDelayNoJitter(10); got != 30*time.Second {
		t.Fatalf("attempt 10 no jitter = %v; want 30s cap", got)
	}
	if got := BackoffDelayNoJitter(-1); got != 1*time.Second {
		t.Fatalf("attempt -1 clamped = %v; want 1s", got)
	}
}

func TestBackoffDelay_JitterRange(t *testing.T) {
	for attempt := 0; attempt <= 5; attempt++ {
		base := BackoffDelayNoJitter(attempt)
		for i := 0; i < 20; i++ {
			delay := BackoffDelay(attempt)
			if delay < base {
				t.Fatalf("attempt %d delay %v < base %v", attempt, delay, base)
			}
			if delay >= base+250*time.Millisecond {
				t.Fatalf("attempt %d delay %v >= base+250ms %v (jitter out of range)", attempt, delay, base)
			}
		}
	}
}

func TestBackoffDelay_ExponentialGrowth(t *testing.T) {
	prev := BackoffDelayNoJitter(0)
	for i := 1; i <= 4; i++ {
		cur := BackoffDelayNoJitter(i)
		if cur <= prev {
			t.Fatalf("backoff not growing: attempt %d %v <= prev %v", i, cur, prev)
		}
		// Should be doubling until cap
		if cur != prev*2 {
			t.Fatalf("attempt %d expected double %v, got %v", i, prev*2, cur)
		}
		prev = cur
	}
}

func TestReadOnlyShim_RespectsHarnessTimeout(t *testing.T) {
	shim := NewReadOnlyShim()
	// Cancelled context should fail immediately — proves ctx cancellation handling
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := shim.Run(ctx, RunSpec{TaskID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled ctx")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}

	// Already-timed-out context via WithTimeout 0 should also fail
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()
	time.Sleep(2 * time.Millisecond) // let it expire
	_, err = shim.Run(ctx2, RunSpec{TaskID: 2})
	if err == nil {
		t.Fatal("expected error for timed-out ctx")
	}
	// Could be DeadlineExceeded or Canceled depending on race
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want DeadlineExceeded or Canceled", err)
	}
}

func TestReadOnlyShim_BudgetDefault(t *testing.T) {
	shim := NewReadOnlyShim()
	// Nil budget should succeed with default handling (no panic)
	_, err := shim.Run(context.Background(), RunSpec{TaskID: 9, TaskName: "budget nil test"})
	if err != nil {
		t.Fatalf("Run with nil budget: %v", err)
	}
	v := 10.0
	_, err = shim.Run(context.Background(), RunSpec{TaskID: 10, TaskName: "budget 10", BudgetUSD: &v})
	if err != nil {
		t.Fatalf("Run with budget 10: %v", err)
	}
}

func TestHarnessTimeoutHelpers(t *testing.T) {
	if HarnessTimeout() != DefaultHarnessTimeout {
		t.Fatalf("HarnessTimeout() = %v; want %v", HarnessTimeout(), DefaultHarnessTimeout)
	}
	if JobLeaseTimeout() != DefaultJobLeaseTimeout {
		t.Fatalf("JobLeaseTimeout() = %v; want %v", JobLeaseTimeout(), DefaultJobLeaseTimeout)
	}
	// WithHarnessTimeout should produce 5m deadline
	ctx := context.Background()
	hCtx, cancel := WithHarnessTimeout(ctx)
	defer cancel()
	if d, _ := hCtx.Deadline(); time.Until(d) < 4*time.Minute || time.Until(d) > 6*time.Minute {
		t.Fatalf("WithHarnessTimeout deadline unexpected: %v", d)
	}
	jCtx, cancel2 := WithJobLeaseTimeout(ctx)
	defer cancel2()
	if d, _ := jCtx.Deadline(); time.Until(d) < 5*time.Minute || time.Until(d) > 7*time.Minute {
		t.Fatalf("WithJobLeaseTimeout deadline unexpected: %v", d)
	}
	// Harness inner must be earlier than job outer when nested
	outer, c1 := WithJobLeaseTimeout(ctx)
	defer c1()
	inner, c2 := WithHarnessTimeout(outer)
	defer c2()
	od, _ := outer.Deadline()
	id, _ := inner.Deadline()
	if !id.Before(od) {
		t.Fatalf("inner %v not before outer %v", id, od)
	}
}
