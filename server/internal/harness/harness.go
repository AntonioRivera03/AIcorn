package harness

import "context"

// RunSpec seeds a harness execution from the current ticket state.
//
// Phase 3: populated from agent_job + task + persona rows. No filesystem.
// Phase 4: adds worktree isolation — caller supplies WorkDir (absolute path
// to a git worktree) and BranchName ("aycorn/task-{id}"). The harness shape
// is: prompt in, working directory, MCP config (AllowedTools + SystemPrompt),
// JSON out, exit code. See Documentation/phase-4-coding-harness.md.
//
// Timeout/budget: the caller nests a timeout via context.WithTimeout before
// calling Run (so the harness deadline fires before the job-lease timeout).
// BudgetUSD is an optional per-job spend cap in USD; nil means use the
// harness default (corresponding to Claude's error_max_budget_usd).
//
// All Phase 4 additions (WorkDir, BranchName, BudgetUSD) are optional and
// zero-safe so the Phase 3 ReadOnlyShim remains valid without them.
type RunSpec struct {
	JobID     int
	TaskID    int
	PersonaID int

	TaskName     string
	TaskBody     string // Plate JSON (already normalized)
	SystemPrompt string
	AllowedTools []string // per-job, per-persona MCP tool allowlist
	Agent        string // persona Agent, e.g. "research", "code-implementation"

	// WorkDir is the absolute path to the git worktree for this job.
	// Empty means no filesystem (Phase 3 shim / dry run).
	WorkDir string

	// BranchName is the git branch for the worktree, e.g. "aycorn/task-123".
	// Empty means the harness chooses its default naming.
	BranchName string

	// BudgetUSD caps spend for this run in USD. Nil means use harness default.
	// Maps to Claude's error_max_budget_usd; the harness enforces it
	// alongside ctx deadline (see Risks in phase-4 doc).
	BudgetUSD *float64

	// MCPConfigPath is the absolute path to the per-job MCP config file
	// generated via mcptools.GenerateMCPConfig. Empty means no MCP config.
	// Set by worker after generation; consumed by OpencodeHarness.Run.
	MCPConfigPath string
}

// RunResult is what the harness produced.
//
// Output is append-only markdown that will be persisted as agent_run.output.
// Summary is a short human-readable title for the run.
// ExitCode follows the process convention: 0 = success, non-zero = failure.
// UsageJson is an opaque JSON blob (token counts, total_cost_usd, usage,
// modelUsage, permission_denials, etc — see Phase 0 spike keys: is_error,
// result, usage, total_cost_usd, permission_denials) stored verbatim.
// Diff is the git diff of the worktree after the run (empty if no changes
// or when running under the Phase 3 shim).
// Truncated indicates Output was truncated to fit storage limits.
type RunResult struct {
	Output    string
	Summary   string
	ExitCode  int
	UsageJson string
	Diff      string
	Truncated bool
}

// Harness executes one agent job synchronously and returns a RunResult.
// The caller (worker) is responsible for claiming/mark-running and for
// persisting the result (agent_run + job status). Harness itself performs
// no DB writes and spawns no external process in the shim case; the real
// harness spawns a subprocess but still performs no DB writes. Harness
// respects ctx cancellation/deadline — caller nests timeout via
// context.WithTimeout and harness returns ctx.Err() promptly when cancelled.
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
// Diff is empty and Truncated is false for the shim (zero-safe).
//
// Timeout nesting: the worker wraps RunOnce with DefaultJobLeaseTimeout (6m)
// while this harness derives an inner context with DefaultHarnessTimeout (5m).
// Because 5m < 6m, the harness deadline always fires first, allowing graceful
// cancellation before the job-lease/StaleTimeout expires. This enforces the
// "Unbounded spend" mitigation (hard context.WithTimeout nested so harness
// timeout fires before job-lease timeout) and is tested in budget_test.go.
//
// Budget: EffectiveBudget(spec) resolves spec.BudgetUSD or DefaultBudgetUSD (5 USD).
// In a real harness this maps to Claude's error_max_budget_usd flag.
func (h *ReadOnlyShim) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	// Derive nested harness timeout — fires before the outer job-lease timeout.
	// If ctx already carries the 6m lease deadline, this inner 5m deadline
	// will be the one that expires first (context nesting = min(deadlines)).
	hCtx, cancel := context.WithTimeout(ctx, DefaultHarnessTimeout)
	defer cancel()

	select {
	case <-hCtx.Done():
		return RunResult{}, hCtx.Err()
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	default:
	}

	// Resolve per-job budget (error_max_budget_usd). Nil => DefaultBudgetUSD.
	_ = EffectiveBudget(spec)

	// Check again after budget resolution (still fast, but respects cancellation).
	select {
	case <-hCtx.Done():
		return RunResult{}, hCtx.Err()
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

	// Final cancellation check before returning (harness must return ctx.Err promptly).
	select {
	case <-hCtx.Done():
		return RunResult{}, hCtx.Err()
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	default:
	}

	return RunResult{
		Output:    output,
		Summary:   summary,
		ExitCode:  0,
		UsageJson: usage,
		Diff:      "",
		Truncated: false,
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
