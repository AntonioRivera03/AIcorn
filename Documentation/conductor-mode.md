# Conductor and independent task sessions

Conductor is the project's fixed dispatcher. It examines tasks explicitly handed to it and starts independent task sessions through Aycorn MCP. It does not perform the task, spawn Codex subagents, move cards, or supervise another conversation's completion. Server code owns the queue, task ownership, stages, and human review handoff.

## Project setup

Project settings → Conductor holds two stage pickers: **While an agent works** and **When the agent finishes**. Neither can be a Done stage, and they can't be the same stage. Conductor works out of the box: a new workspace's starter workflow, and every new custom workflow, ship as Open, Doing, Review, Done, so "Doing" and "Review" are already there. An existing workflow that never had a saved choice gets one automatically — the first Doing stage, and the first later stage that is neither Open nor Done, preferring one whose name looks like a review stage — shown in Settings as editable defaults and saved the first time Conductor actually needs them (starting, sending a task, or a Job). A workflow with a working stage but nothing to hand finished work to (the classic Open, Doing, Done) can't resolve a finish stage; Settings and the board explain that and offer a one-click "Add a Review stage" that inserts one right after the working stage.

The agent's working and handoff instructions are built into Aycorn, not project settings — Conductor's own fixed instructions already say how to choose tasks. The only remaining project setting is optional: extra guidance for choosing which tasks are ready, plus whether tasks use the linked repository. Agent models are in Settings → Aycorn AI, not a dedicated AI page; bundled instructions and skills there are read-only.

Send selected tasks to Conductor from the board or list. A send always queues the task even when Conductor can't run yet — the board's toast and Aycorn MCP's `send_to_conductor` result say why (not configured, or paused) so nothing silently sits idle. Turning Conductor on permits selection and queued work to start. Pausing stops new dispatch/start operations; an already-running task can finish. Scheduled Jobs and a user's confirmed request for more work are explicit operations and do not depend on this board toggle.

## Dispatch and execution

1. The worker opens a short project-level Codex dispatcher session in an empty directory. Its MCP catalog contains exactly `list_conductor_tasks`, `start_conductor_task`, `defer_conductor_task`, `project_context`, and `read_project_document`. Inherited MCP connections, apps, shell execution, web search, and native agent delegation are disabled for this dispatcher.
2. Conductor reads the available context and chooses a role: `coder`, `researcher`, or `reviewer`. It calls `start_conductor_task` with the project ID, task ID, and role. It can defer a task with a concrete blocker.
3. The server validates project scope, the active dispatcher, Conductor ownership, stage configuration, dependencies, and task contents. One SQLite transaction moves the task to In progress, assigns the selected agent, and queues its work. Duplicate starts return the existing queued/running job.
4. The worker launches the selected agent as the root of its own persistent Codex session. The role's fixed instructions, selected model, workflow skill, scoped task/project MCP context, and task objective are supplied programmatically. Coder work with a repository uses an isolated worktree; research/review sessions are read-only. Native subagents remain disabled.
5. A successful work turn must return a validated structured handoff with `completed: true`. The controller preserves the user's task description, appends the handoff, and moves the task to Human review. Failures, cancellations, blockers, malformed answers, changed task content, and lost ownership do not enter review. Nothing goes directly to Done.

The worker still executes one queued task turn at a time. Sessions are independent and persistent; this change does not add a parallel worker pool. There is no automatic merge, push, deployment, or paid retry loop. Unselected tasks wait for an explicit recheck, and interrupted runs retain their history instead of silently replaying.

## Continuing a task

Open Task AI (or the ticket chat for a Chat-type task) to see its requests, responses, artifacts, and run history. Its composer resumes the saved Codex thread and worktree.

- A confidently classified **question** runs read-only and leaves the stage unchanged. Shell execution, web search, write MCP tools, and native delegation are unavailable for that turn.
- A request for **more work** opens a confirmation dialog. **Cancel** sends nothing. **Ask only** submits a question-only turn. **Resume work** atomically moves a managed task back to its captured In progress stage and queues work in the same session.
- The chat is locked while queued/running/canceling. The backend also enforces single ownership, idempotency, and the latest conversation/stage cursor, including when two tabs submit together.
- If the first run failed before a Codex thread existed, confirming Resume work creates the missing session. A failed provider call cannot permanently strand the composer.

The router uses Jev only when `TYPESAFE_API_KEY` is configured on the server. Without it, or if classification is ambiguous/unavailable, the dialog asks the user to select question or work. See [intent routing](task-intent-routing.md) for setup, API contract, and research.

## Goal-style execution and persistence

Each work turn receives instructions to pursue the complete task objective, verify the result, make reasonable decisions, and report genuine blockers. Aycorn uses the [Codex app-server thread and turn APIs](https://learn.chatgpt.com/docs/app-server): `thread/start`, `thread/resume`, and `turn/start`. It does **not** call native `thread/goal/set`: the installed driver starts an autonomous turn immediately on that call, before Aycorn can submit its task input and structured output schema. Aycorn must own turn startup and termination to keep locking and review correct.

The existing timeout remains the execution limit. A timeout does not count as completion. The persisted thread ID, turn ID, worktree, branch, base commit, and artifacts support explicit continuation.

## Storage and compatibility

Migration `00027_task_sessions.sql` adds dispatcher history, a handoff token protecting release/re-send races, and a task-message idempotency index. It updates only the old default selection prompt; customized prompts are retained. Existing `task.priority`, `task.type`, and `stage.type` CHECK constraints are unchanged. The former planning-stage field stays in stored settings for compatibility but is no longer required or shown.

`request.taskSession` records the role, question/work mode, and captured stage contract. `request.chat` retains the persistent session/worktree cursor. Old run history remains visible. Already queued legacy planning runs can finish their readiness check; their next execution uses a direct task agent. New scheduled Jobs immediately queue their selected independent task agent.

## Verification

On 2026-09-16, `TestInstalledConductorSessionsAndMCP` passed against the installed Codex driver and a disposable database. Actual model/tool calls selected a research task through MCP, moved it to its working stage, executed it in a distinct task thread, retrieved a unique project-document token, and answered a follow-up in the same task thread. Native histories contained no subagent spawning. The conversations were archived after testing.

Automated regressions cover scope, revocation, duplicate starts/messages, blockers, pause/claim races, canceled/failed/interrupted runs, stale task edits, manual stage changes, confirmation, question/work separation, and recovery when a thread was never created. The standard checks are the full Go suite, frontend tests/build, focused lint, and `make check-agents`.
