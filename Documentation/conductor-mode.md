# Conductor mode

The local app-server integration and agent fleet are documented in [Local harnesses](local-harnesses.md). Conductor now stays the root agent throughout a cycle; the selected task agent configures its coder subagent.

Conductor manages selected tasks on the project's existing board. The List and Kanban views share an animated orange frame and a **CONDUCTOR MANAGING** label. A Conductor-only filter provides a focused view of the same tasks, without copying them to another board or changing their checklist.

## Using it

1. On the **AI** page, create custom agents. Edit each name, OpenAI model, and rich-text instructions in place; changes autosave. Sign in to Codex on the server with `codex login`.
2. Open **Project Settings → Conductor**. Choose three distinct stages from the project's workflow: planning, in progress, and human review. Done stages are excluded from all three roles.
3. Edit each stage's prompt in place. The completion prompt tells the worker what to include in its handoff. Changes save on blur or selection.
4. Select a **Conductor agent** and **Task agent** from your custom agents. Both selections are required before enabling Conductor. They may use different OpenAI models; the same agent can fill both roles if desired.
5. Choose whether workers should use the project's repository. Repository tasks use the existing isolated worktree runner. Written-answer tasks can run without a repository.
6. Use **Send to Conductor** in a task's menu, right-click menu, or selection toolbar. Start Conductor when ready. Sending while paused just marks the task for later.

The menus are accessible through their buttons and through Shift+F10. The animated border respects reduced-motion preferences. Orange is defined through the `conductor` semantic color token for both light and dark themes.

## Lifecycle and ownership

| State | Behavior |
| --- | --- |
| Waiting | Selected by the user; awaits planning while the project is enabled. |
| Planning | A read-only orchestration session checks context and acceptance criteria. |
| Needs context | Specific questions or blockers are shown on the task. Planning questions and worker handoffs are appended to the body. Add information and explicitly choose Recheck. |
| Queued | Conductor has appended its plan and assigned your task agent. The task stays in planning until the worker starts. |
| Working | Starting the queued worker atomically moves the task into its configured in-progress stage. |
| Human review | A successful, complete worker result is appended to the task body and the task moves to its configured handoff stage. |
| Failed / Held | Provider errors, interruption, invalid decisions, changed workflow, or manual changes require attention and explicit recheck. |

A successful process exit alone does not complete a task: the worker must return a valid structured handoff stating that the task is complete. An incomplete result stays out of review. Unresolved blocking relationships prevent implementation from starting.

**Pause** prevents new planning and queue starts. A session already running may finish; a finished planner waits for resume before queuing implementation. **Release** removes Conductor ownership and cancels its pending or active run. Existing output and worktrees remain available in the AI panel.

Manual stage changes take precedence over automatic transitions. Editing content during planning or before a worker starts requires a recheck. A human moving reviewed work onward clears its Conductor badge while preserving run history. Conductor never moves a task to a Done stage and never merges a branch.

The selected agents’ names, models and converted instructions, settings, and stage contract are frozen for each planning cycle. Later edits apply to the next cycle; stages are still checked against the project's current workflow before execution and handoff. All task, stage, body, queue, and assignment changes at a handoff commit together. The persisted job cursor prevents duplicate notes or duplicate execution after a restart. Interrupted executions require explicit recheck.

Body updates append new Plate nodes, preserving existing formatting. The editor sends an `If-Match` body revision; a stale editor receives a conflict instead of silently overwriting a new handoff. Copy unsaved edits and reopen the task when a conflict is reported.

## Integration choice

The implementation uses a **durable state machine with structured agent decisions** and a **Codex adapter** behind the existing harness interface. The job queue, worktrees, and history remain owned by Aycorn.

Codex runs each job through `codex exec --json`, with a JSON output schema for Conductor phases. The final-message file separates the answer from intermediate progress, and both a completed turn and a successful process exit are required. The built-in OpenAI provider chooses the appropriate endpoint for saved ChatGPT sign-in or API-key authentication. No alternate provider or simulated production fallback is used. [Codex noninteractive mode](https://learn.chatgpt.com/docs/non-interactive-mode), [Codex authentication](https://learn.chatgpt.com/docs/auth)

The planner's MCP server exposes `search_tasks` and `read_task`, scoped server-side to the current project. Workers receive only `read_task` for their own task. These read tools are explicitly approved for unattended execution. Agents return a readiness decision or completion handoff; the backend validates it and commits task/queue/stage changes together. This keeps live context in MCP and durable workflow rules in application code. [Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)

I considered Codex App Server and a direct Responses API integration. App Server offers richer session and account APIs, but the existing job lifecycle only needs one bounded run with progress and a final result. The direct API would require rebuilding command execution and workspace management. Noninteractive Codex therefore fits this release with less machinery; the harness interface leaves room for App Server later. [Codex App Server](https://learn.chatgpt.com/docs/app-server)

User config and exec policy files are ignored for managed runs; repository config is untrusted. Explicit process overrides select the OpenAI provider, scoped MCP, disabled apps/plugins/subagents, and the sandbox. Planning and other non-implementation intents use read-only mode. Repository implementation uses workspace-write with a private temporary/cache directory, and can execute commands/tests. Commands do not inherit provider keys or Aycorn database settings; network access is disabled. The sandbox is not a task container: dependencies must already be available locally, and unavailable downloads must be reported honestly. [Codex configuration](https://learn.chatgpt.com/docs/config-file/config-reference)

Custom agents reuse the existing persona storage and endpoints. This preserves IDs, prompts and run history while changing the UI vocabulary. The historical role/tool-list fields remain stored for compatibility but do not grant runtime permissions; intent and Conductor phase determine access. Model inputs suggest OpenAI models and accept newer OpenAI model IDs without a schema change. Actual account/model access is determined by Codex.

## Schema and API

Migration `00016_conductor.sql` adds `conductor_project` and `conductor_task`. It does not alter the allowed values of `stage.type`, `task.priority`, or `task.type`.

Migration `00017_codex_agents.sql` preserves custom names/prompts and historical run snapshots, converts agent harnesses to Codex, preserves recognized OpenAI model names, and assigns `gpt-5.6-sol` to prior non-OpenAI choices. It clears the old executable setting, interrupts old queued/running jobs, pauses Conductor, and requires selecting custom agents before restarting. Persona model/harness columns are plain TEXT; no CHECK-constraint rebuild is needed.

- `GET /api/project/{projectId}/settings/conductor`: settings, task states, and configuration problems.
- `PUT /api/project/{projectId}/settings/conductor`: validated field patches, with optimistic protection against competing settings edits.
- `POST /api/project/{projectId}/conductor/bulk`: `{ids, action}`, where action is `send`, `recheck`, or `release`. One transaction returns the standard `BulkResult`; cross-project, done, active, or already-managed items are skipped as appropriate.

The UI shares one Conductor query per board. Cursor changes refresh task positions and AI results; active body editors keep their own revision until a successful save or reopen.

## Validation and current limits

Automated coverage includes planning → queue → execution → review; missing context; body preservation and concurrent edits; pause/release races; manual stage moves; restart recovery; provider failures and malformed results; unexpected model-assignment fields; dependencies and deleted stages; bulk and MCP project scope; MCP result-schema validation; settings patches; stale body saves; Codex process cancellation; migration preservation; and agent edits/deletion during an active cycle. The full Go suite, focused race tests, 75 frontend tests, production build, and focused lint checks are the acceptance checks.

A live acceptance run used Codex CLI 0.153.4, a custom planning agent on `gpt-6-astra`, and a task agent on `gpt-5.6-sol`, with a disposable database and tiny Git repository. The planner read MCP task/project context and source files. The worker changed subtraction to addition, ran `node --test sum.test.cjs` successfully, and returned its structured handoff. Aycorn preserved the one-file patch, appended the summary, and moved the ticket to In review. An independent test rerun passed; the original checkout and test file remained unchanged. Nothing was merged or pushed.

The live test also caught two integration details: forcing the API URL breaks saved ChatGPT authentication, and unattended read tools need explicit MCP approval settings. Regression checks cover the final configuration. Browser checks verified custom agent creation, name/model/prompt autosave, project agent selection, and the existing board. An existing hook-order crash in General settings was repaired because it prevented opening Project Settings on a fresh load.

One agent runs at a time. There is no automatic retry, automatic merge, spending cap, or parallel worker pool. Codex execution remains on the host with command networking disabled. [Kubernetes environments](kubernetes-architecture.md) provide isolated test and application containers, including optional previews after successful Conductor code completions; they do not containerize the agents themselves. The [branch environments and task containers specification](branch-environments-spec.md) also describes future execution profiles. Manual runs still do not control workflow stages. Conductor never moves work directly to Done.
