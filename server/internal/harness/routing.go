package harness

import (
	"context"
	"strings"
)

type RoutingHarness struct {
	Shim *ReadOnlyShim
	Real *OpencodeHarness
}

func NewRoutingHarness(shim *ReadOnlyShim, real *OpencodeHarness) *RoutingHarness {
	return &RoutingHarness{Shim: shim, Real: real}
}

func (r *RoutingHarness) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	if r.shouldUseReal(spec) {
		return r.Real.Run(ctx, spec)
	}
	return r.Shim.Run(ctx, spec)
}

func (r *RoutingHarness) shouldUseReal(spec RunSpec) bool {
	// Precise: Agent is the source of truth. Research stays shim (fast, no spend).
	if strings.EqualFold(spec.Agent, "research") {
		return false
	}
	if strings.EqualFold(spec.Agent, "code-implementation") || strings.EqualFold(spec.Agent, "general-senior") || strings.EqualFold(spec.Agent, "code-analysis") {
		return true
	}
	// Fallback for older specs without Agent: use SystemPrompt or write-tool presence
	if strings.Contains(strings.ToLower(spec.SystemPrompt), "coding") {
		return true
	}
	for _, t := range spec.AllowedTools {
		if t == "create_task" || t == "update_task" {
			return true
		}
	}
	if len(spec.AllowedTools) > 3 {
		return true
	}
	return false
}
