package harness

import "context"

// RunSpec seeds a harness execution from the current ticket state.
// Phase 3: populated from agent_job + task + persona rows. No filesystem.
type RunSpec struct {
	JobID       int
	TaskID      int
	PersonaID   int
	TaskName    string
	TaskBody    string // Plate JSON (already normalized)
	SystemPrompt string
	AllowedTools []string
}

// RunResult is what the harness produced.
// Output is append-only markdown that will be persisted as agent_run.output.
// Summary is a short human-readable title for the run.
// ExitCode follows the process convention: 0 = success, non-zero = failure.
// UsageJson is an opaque JSON blob (token counts, etc) — stored verbatim.
type RunResult struct {
	Output    string
	Summary   string
	ExitCode  int
	UsageJson string
}

// Harness executes one agent job synchronously and returns a RunResult.
// The caller (worker) is responsible for claiming/mark-running and for
// persisting the result (agent_run + job status). Harness itself performs
// no DB writes and spawns no external process.
type Harness interface {
	Run(ctx context.Context, spec RunSpec) (RunResult, error)
}

// ReadOnlyShim is the Phase 3 fake harness. It simulates a fresh harness
// process seeded from ticket state by returning deterministic markdown without
// touching the filesystem, spawning a subprocess, or calling an LLM.
//
// Output is suitable for persisting as agent_run.output and demonstrates the
// plumbing: harness reads ticket, writes markdown into agent_run, job completes.
type ReadOnlyShim struct{}

// NewReadOnlyShim creates a ReadOnlyShim. No config needed for Phase 3.
func NewReadOnlyShim() *ReadOnlyShim { return &ReadOnlyShim{} }

// Run returns a canned markdown payload. It respects ctx cancellation.
func (h *ReadOnlyShim) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	select {
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	default:
	}

	// Keep output deterministic for tests but include the key seed fields
	// so a human can see the plumbing is live.
	output := "# Research notes for task " + itoa(spec.TaskID) + ": " + spec.TaskName + "\n\n" +
		"This is a read-only shim. No filesystem was touched.\n\n" +
		"## Ticket\n\n" +
		"- **Task ID:** " + itoa(spec.TaskID) + "\n" +
		"- **Job ID:** " + itoa(spec.JobID) + "\n" +
		"- **Persona ID:** " + itoa(spec.PersonaID) + "\n" +
		"- **Allowed tools:** " + toolsString(spec.AllowedTools) + "\n\n" +
		"## What a real harness would do here\n\n" +
		"- read via `search_tasks` / `list_projects` / `read_task`\n" +
		"- inspect codebase / docs (read-only)\n" +
		"- write findings as markdown into `agent_run.output`\n\n" +
		"## Seeded body (truncated)\n\n" +
		truncate(spec.TaskBody, 500) + "\n"

	summary := "research notes for task " + itoa(spec.TaskID)
	usage := `{"shim":true,"persona_id":` + itoa(spec.PersonaID) + `}`

	return RunResult{
		Output:    output,
		Summary:   summary,
		ExitCode:  0,
		UsageJson: usage,
	}, nil
}

// FakeHarness is an alias for ReadOnlyShim kept for backwards compat with
// the task description which mentions either name.
type FakeHarness = ReadOnlyShim

// NewFakeHarness creates a shim under the alternate name.
func NewFakeHarness() *FakeHarness { return NewReadOnlyShim() }

func itoa(n int) string {
	// avoid importing strconv in the hot path for trivial ints; use fmt-free version
	// but strconv is fine and clearer.
	return intToString(n)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func toolsString(tools []string) string {
	if len(tools) == 0 {
		return "(none — read-only)"
	}
	out := ""
	for i, t := range tools {
		if i > 0 {
			out += ", "
		}
		out += "`" + t + "`"
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
