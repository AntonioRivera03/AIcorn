# Phase 4 — The Coding Harness

Status: Not started

The actual coding capability. Isolated blast radius by design — this is the first phase where an agent can write to the filesystem.

---

## Ships

- `server/internal/harness/harness.go` — a narrow `Harness` interface.
- `ClaudeCodeHarness` — the first (and for now, only) implementation.
- Git-worktree-per-job execution.
- A "Coder" persona with write access, gated by per-job MCP tool config.
- Run output / diff view in the task drawer.

## Why here

OpenCode, the harness originally in mind, is **archived** (superseded by Crush, whose programmatic API isn't well documented). Every viable harness — Claude Code, Crush, Aider, Cline, Goose — exposes roughly the same shape: prompt in, working directory, MCP config, JSON out, exit code. So build one narrow interface and one concrete implementation, and keep the door open for a second later:

```go
type Harness interface {
    Run(ctx context.Context, spec RunSpec) (RunResult, error)
}

// ClaudeCodeHarness is the first (and for now, only) implementation.
```

`ClaudeCodeHarness` invokes `claude -p` with `--output-format json`, `--bare` (skips hook/plugin loading, for deterministic runs), and `--worktree` — or the Agent SDK directly if streaming progress matters. Crush becomes a second implementation later, once its programmatic surface is verified by hand (public docs are too thin to plan against today).

## Build — job lifecycle

1. Create a worktree on branch `aycorn/task-{id}` — shared `.git` object store, no file conflicts, the established per-job isolation pattern.
2. Run the harness with the persona's `allowed_tools` compiled into its MCP config for that specific job, so a Research persona is physically incapable of write access even via a prompt-injection path.
3. Capture the diff, write it to `agent_run`, transition the ticket to Review (Phase 0 endpoint).
4. Leave the branch for human inspection. **Do not auto-merge** — merge is the consequential, irreversible action; that's exactly where the human gate belongs.

## Risks

**Rate limits.** Claude Code returns 529s past 3–4 concurrent session starts. Mitigate with `maxConcurrentJobs = 1` to start, and exponential backoff on 529 rather than immediate retry.

**Unbounded spend.** "No token budget or timeout" is a named anti-pattern with real production spend incidents behind it. Claude Code surfaces `error_max_budget_usd` — set a per-job budget and a hard `context.WithTimeout` on the `exec.Command`, nested so the harness timeout fires before the job-lease timeout.

**Over-broad tool scope.** A hallucinated argument to an unrestricted tool is the mechanism behind the worst documented multi-agent incidents. Per-job, per-persona MCP config generation (step 2 above) exists specifically to prevent this — don't skip it to save time.

## Done when

- [ ] `Harness` interface defined, `ClaudeCodeHarness` implemented and tested against a real repo.
- [ ] Worktree created/cleaned up correctly per job.
- [ ] Coder persona's tool access verified as actually restricted (test: try to make it call a tool outside its `allowed_tools`).
- [ ] Per-job budget and nested timeouts enforced.
- [ ] Diff/output visible in the task drawer; merge remains a manual human action.
