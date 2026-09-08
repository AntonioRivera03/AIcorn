**Task AI redesign — implementation and verification**

> Historical phase-004 implementation notes. The Conductor branch replaces the OpenCode adapter with Codex, adds custom agent models and sandboxed commands/tests, and preserves the task branch/merge work described here. See [Conductor mode](conductor-mode.md) for current behavior and validation.
The replacement described in [the review](ai-integration-redesign-review.md) is implemented on `phase-004`. The original review is a dated baseline; its defect descriptions and initial test failures describe the previous implementation.

**What changed**

- An explicit Run action is the execution trigger. Task creation, assignee edits, stage transitions, and bulk moves do not enqueue AI work. Existing owners remain untouched. Old stage-persona bindings remain stored but inactive.
- One shared task AI panel serves task drawers, full task pages, and board/list actions. Ask, Plan, Implement, and Review select an intent. Ask/Plan can run on task context alone; repository access is visible and optional unless code is required.
- The panel shows readable Markdown, progress and elapsed time, Stop, request reuse, all attempt outcomes, and real file patches. Technical metadata is collapsed. Board/list indicators are quiet, clickable, and retain terminal outcomes. Keyboard entry is available through the header, Ctrl/Command+Shift+A, and the command palette.
- Personas are optional instruction presets. The preset editor exposes names and instructions; engine/model settings live under AI. Old runtime fields remain in storage for compatibility and do not control execution.
- OpenCode is the sole adapter. There is no simulated production fallback, vendor guessing, prompt-based routing, or automatic retry. Resolved requests retain their model, executable/version, task text, optional preset text, intent, repository and timeout.
- MCP exposes only `read_task` to an AI run, and rejects another task ID. It receives an absolute executable and the exact absolute database path. Ordinary external MCP clients retain the existing tool catalog.
- Native permissions are deny by default. Repository analysis can read/search; Implement can edit. Shell, subagents, external directories, automatic formatters and language servers are disabled. This is an engine permission policy, not an OS sandbox.
- Runs have atomic active-job uniqueness, a database worker lock, process-tree cancellation, checkpoints, and explicit interrupted outcomes on restart. Uncertain work is never silently replayed.
- Workspaces have random run identities and are never automatically reused, pruned or deleted. Capture includes new file contents and committed edits against the recorded base, using a separate Git index. Failure/cancellation retains partial output and the workspace.
- Migration 15 preserves legacy jobs and outputs, makes preset references nullable, interrupts old queued work, and adds explicit request/settings/artifact fields. Deleting a preset preserves its history. No task priority/type or stage type CHECK values changed.
- Removed obsolete routing, fake harness, budget/retry, temporary MCP config, worker execution paths, duplicate query hooks and unused stage-persona UI. Tests asserting the old behavior were replaced with regressions for the new contract.
- Development/build/install targets build the MCP companion; release CI includes matching companion assets. Node.js is required for Markdown conversion and checked in engine readiness.

**What was verified**

| Check | Evidence |
|---|---|
| Dependencies | `npm install` completed. Existing npm audit findings remain; no unrelated dependency upgrade was attempted. |
| Automated tests | `make test`: all Go packages and 9 frontend test files / 75 tests pass. |
| TypeScript | `make typecheck` passes. |
| Focused lint | AI, job indicator, and preset feature directories pass ESLint. |
| Build | Frontend development build and MCP binary build pass. Windows web/MCP cross-compilation checked separately. |
| Lifecycle | Concurrent submissions create one active request; concurrent claims execute once; claimed cancellation never starts the engine; running cancellation/shutdown retains output; restart interrupts existing work without replay. |
| Artifacts | Failure preserves a new file and patch; capture includes committed and untracked content without altering the real index; identity collision does not damage an existing workspace. |
| Migration | A populated version-14 database retains job IDs, run IDs, output, and legacy usage through migration and persona deletion; foreign-key check passes. |
| Scoped tools | In-memory MCP client sees only `read_task`; a forbidden write tool and out-of-scope task read are rejected. |
| Real Ask | OpenCode 1.18.29, configured model `opencode-go/muse-spark-1.2-contributor`, read disposable task 1 and returned `AYCORN_SMOKE_OK BLUEBIRD`. |
| Real Implement | Created `smoke-result.txt` in a disposable worktree; independently verified its file content and captured patch. |
| Real boundary test | Created `boundary-ok.txt` inside the worktree; outside-directory write reported policy rejection and the outside file was absent. |
| Browser | Not verified. Computer-use inventory repeatedly returned no enabled browser. No screenshot or rendered interaction claims are made. |

The real tests used `/tmp/aycorn-ai-redesign-smoke.db` and `/tmp/aycorn-ai-implement-smoke`. No AI prompt was submitted for a personal task. The disposable artifacts are left available for inspection.

One live test caught a defect simulated tests missed: OpenCode preferred inherited `PWD` over the subprocess working directory, and wrote the smoke file in Aycorn's server directory. The exact cause is visible in [OpenCode 1.18.29's run command](https://github.com/anomalyco/opencode/blob/v1.18.29/packages/opencode/src/cli/cmd/run.ts#L322). The adapter now pins both `PWD` and `--dir`; a regression test enforces their agreement. The misplaced test file was removed, and subsequent real runs wrote only inside the correct workspace. Model claims alone were insufficient evidence of successful artifact capture.

**Remaining validation and deliberate limits**

The browser walkthrough remains a required sign-off: drawer/full page, keyboard focus and Escape, request/cancel/reuse, empty/error states, narrow viewport, and light/dark presentation. Builds and source review cannot substitute for that visual check.

This version does not run tests on the user's behalf, modify task text automatically, or delete agent workspaces. The subsequent task-branch feature adds explicit local Git merges through the task's Code branches section (see README). Review examines a fresh checkout of the linked repository; it does not yet select a previous Implement workspace. Request reuse prepares a new request using current task context/settings. Reported token/cost usage accumulates across steps; there is no dollar spending cap.

Progress and partial answers are checkpointed, but a complete raw provider-event archive and linked-task context selection remain follow-up work. Stage automation, persona handoffs, multiple workers, automatic merging, and workspace cleanup controls remain deferred as proposed in the redesign.

The next pass should begin with the actual browser walkthrough and refine the UI against what it reveals. After that, the next useful capability is reviewing a selected implementation's changes and explicitly applying useful text to a task—not restoring implicit stage automation.
