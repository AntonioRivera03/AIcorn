package harness

import (
	"context"
	"fmt"
)

type ProviderInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

func Providers() []ProviderInfo {
	return []ProviderInfo{{ID: "codex", Name: "Codex", Enabled: true}, {ID: "opencode", Name: "OpenCode", Reason: "Coming later"}}
}

// Registry is the only production dispatch path. A saved/forged OpenCode request
// cannot bypass the UI's disabled option and start a process.
type Registry struct{ Codex *Codex }

func (r *Registry) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	if spec.Request == nil {
		return RunResult{}, fmt.Errorf("missing harness request")
	}
	switch spec.Request.Engine {
	case "codex":
		if r.Codex == nil {
			return RunResult{}, fmt.Errorf("Codex adapter is not configured")
		}
		return r.Codex.Run(ctx, spec)
	case "opencode":
		return (&OpenCode{}).Run(ctx, spec)
	default:
		return RunResult{}, fmt.Errorf("unknown harness %q", spec.Request.Engine)
	}
}

// OpenCode reserves the provider boundary for its local HTTP/SSE server. It is
// intentionally unavailable until its session, permission and event mapping is
// implemented and tested; it never silently falls back to Codex.
type OpenCode struct{}

func (*OpenCode) Run(context.Context, RunSpec) (RunResult, error) {
	return RunResult{}, fmt.Errorf("OpenCode is disabled; select Codex")
}
