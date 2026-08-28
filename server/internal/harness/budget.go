package harness

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// Budget and timeout controls for harness jobs.
//
// Risks (Documentation/phase-4-coding-harness.md):
//   - Rate limits: Claude returns 529 past 3-4 concurrent session starts.
//     Mitigate with maxConcurrentJobs=1 and exponential backoff on 529.
//   - Unbounded spend: "No token budget or timeout" is a named anti-pattern.
//     Enforced via per-job error_max_budget_usd (BudgetUSD) and hard
//     context.WithTimeout nesting so the harness deadline fires before the
//     job-lease deadline.
//
// Timeout nesting invariant:
//   DefaultHarnessTimeout (5m) < DefaultJobLeaseTimeout (6m).
//   Worker wraps RunOnce's outer ctx with DefaultJobLeaseTimeout (6m) while
//   Harness.Run internally derives a child ctx with DefaultHarnessTimeout (5m).
//   The harness timeout therefore always fires first, allowing graceful
//   cancellation before the lease (StaleTimeout 5m / job claimedAt) expires.
//
// Budget:
//   spec.BudgetUSD maps to Claude's error_max_budget_usd flag. Nil means
//   use DefaultBudgetUSD (5 USD). Harness implementations should pass
//   EffectiveBudget(spec) to the underlying CLI/SDK.

const (
	// DefaultBudgetUSD is the per-job spend cap in USD when spec.BudgetUSD is nil.
	// Corresponds to Claude's error_max_budget_usd.
	DefaultBudgetUSD = 5.0

	// DefaultHarnessTimeout is the hard timeout for a single harness execution.
	// The harness derives a child context with this timeout; it must be
	// strictly less than DefaultJobLeaseTimeout so it fires first.
	DefaultHarnessTimeout = 5 * time.Minute

	// DefaultJobLeaseTimeout is the outer worker lease timeout for RunOnce.
	// Must be greater than DefaultHarnessTimeout (nested timeout invariant).
	DefaultJobLeaseTimeout = 6 * time.Minute

	// MaxBackoff is the cap for exponential backoff on rate-limit errors.
	MaxBackoff = 30 * time.Second

	// MaxRateLimitRetries is the number of retries on 529/rate-limit before marking failed.
	MaxRateLimitRetries = 3
)

// budgetCtxKey is the context key for per-job budget overrides.
type budgetCtxKey struct{}

// WithBudget returns a child context carrying the per-job budget override.
// This is for harnesses that read budget from context rather than RunSpec.
func WithBudget(ctx context.Context, budget float64) context.Context {
	return context.WithValue(ctx, budgetCtxKey{}, budget)
}

// BudgetFromContext extracts a budget previously stored via WithBudget.
// Returns 0, false if not present.
func BudgetFromContext(ctx context.Context) (float64, bool) {
	v, ok := ctx.Value(budgetCtxKey{}).(float64)
	return v, ok
}

// EffectiveBudget returns the budget that should be used for spec.
// If spec.BudgetUSD is nil or non-positive, DefaultBudgetUSD is returned.
func EffectiveBudget(spec RunSpec) float64 {
	if spec.BudgetUSD != nil && *spec.BudgetUSD > 0 {
		return *spec.BudgetUSD
	}
	return DefaultBudgetUSD
}

// IsRateLimitErr reports whether err is a rate-limit / 529 error.
// Checks for the substrings "529", "rate limit" (case-insensitive),
// "rate_limit", and "api_error_status" which appears in Claude JSON errors.
// A nil error returns false.
func IsRateLimitErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(msg, "529") {
		return true
	}
	if strings.Contains(lower, "rate limit") {
		return true
	}
	if strings.Contains(lower, "rate_limit") {
		return true
	}
	if strings.Contains(lower, "api_error_status") {
		return true
	}
	return false
}

// BackoffDelay returns the exponential backoff duration for the given retry attempt (0-indexed).
// Formula: min(2^attempt * 1s, MaxBackoff) + jitter[0,250ms).
// Attempt is clamped to >=0. The jitter prevents thundering-herd retries.
func BackoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	// Exponential: 2^attempt seconds, capped at MaxBackoff.
	base := time.Duration(1<<uint(attempt)) * time.Second
	if base > MaxBackoff {
		base = MaxBackoff
	}
	// Jitter 0-250ms. Use global rand (seeded by runtime); deterministic range is all that tests assert.
	jitter := time.Duration(rand.Intn(250)) * time.Millisecond
	return base + jitter
}

// BackoffDelayNoJitter returns the deterministic base delay without jitter.
// Useful for testing exact intervals.
func BackoffDelayNoJitter(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := time.Duration(1<<uint(attempt)) * time.Second
	if base > MaxBackoff {
		base = MaxBackoff
	}
	return base
}

// HarnessTimeout returns the timeout that Harness.Run will enforce internally.
func HarnessTimeout() time.Duration { return DefaultHarnessTimeout }

// JobLeaseTimeout returns the outer worker lease timeout.
func JobLeaseTimeout() time.Duration { return DefaultJobLeaseTimeout }

// WithHarnessTimeout derives a child context with DefaultHarnessTimeout.
// Caller must call cancel(). This is the inner/nested timeout that fires first.
func WithHarnessTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultHarnessTimeout)
}

// WithJobLeaseTimeout derives a child context with DefaultJobLeaseTimeout.
// Caller must call cancel(). This is the outer lease timeout (worker loop).
func WithJobLeaseTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultJobLeaseTimeout)
}
