# Built-in agents

Every project has a fixed **Conductor** dispatcher. **Coder**, **Research**, **Reviewer**, and **Planner** run tasks in separate persistent sessions. **Chatter** handles the project conversation. Native orchestrator/subagent delegation is disabled.

On the **AI** page, the only editable agent property is the OpenAI model. Model changes autosave on blur or Enter. Names, roles, prompts, skill references, and permissions are read-only. Agent details show the bundled Markdown and workflow skill. Historical custom agents retain their saved definitions for existing task and Job references; only their model remains editable.

## One definition for UI and execution

Canonical instructions live in `server/internal/harness/fleet/`. The persona read path overlays these definitions onto stored model preferences. The harness materializes a content-addressed fleet directory beside the database and includes the workflow skill in each session.

Repository `.codex/agents/` Markdown and TOML files mirror the catalog. `.codex/config.toml` registers them with native delegation disabled. Research retains the repository filename `research.toml` and runtime role `researcher`.

After changing canonical instructions:

```sh
make sync-agents
make check-agents
```

Rebuild/restart Aycorn to load changed instructions. Model preferences belong to SQLite; editing a model does not rewrite repository files. Each task's initial request captures its agent/model and stage contract; follow-up turns retain that model and resume its session.

## Roles and tools

| Agent | Responsibility | Execution |
| --- | --- | --- |
| Conductor | Inspect managed tasks, select a role, start or defer tasks through restricted MCP | Separate project dispatcher; no shell, web, repository writes, or subagents |
| Coder | Implement and verify the assigned task | Independent task session; isolated writable worktree when repository use is enabled |
| Research | Investigate sources and produce grounded findings | Independent read-only task session |
| Reviewer | Inspect work, correctness, and validation; report findings | Independent read-only task session |
| Planner | Produce actionable plans and acceptance criteria | Independent read-only task session |
| Chatter | Discuss the project and use its scoped project tools | Existing persistent project conversation |

Conductor's five tools are `list_conductor_tasks`, `start_conductor_task`, `defer_conductor_task`, `project_context`, and `read_project_document`. `start_conductor_task(projectId, taskId, role)` calls the same server services that own task transitions and queue insertion. Conductor cannot pass arbitrary prompts, models, workspaces, or session IDs.

Task agents receive assigned-task/project MCP context. Server-side scope and ownership checks remain authoritative. Questions restrict the tool allowlist further and run read-only. Project documents include written content and attachment metadata; these MCP reads do not claim to extract binary attachment text or OCR.

Read [Conductor and task sessions](conductor-mode.md) for lifecycle, follow-up confirmation, limits, and migration details.

## Real driver acceptance test

Build the MCP executable (`make build-mcp`), then run:

```sh
cd server
AYCORN_CONDUCTOR_LIVE=1 \
AYCORN_CODEX_INTEGRATION="$(command -v codex)" \
AYCORN_MCP_INTEGRATION="$PWD/bin/aycorn-mcp" \
go test ./internal/harness -run '^TestInstalledConductorSessionsAndMCP$' -count=1 -v
```

This uses the local Codex login for real model turns. It verifies restricted dispatch, a separate task thread, project-document MCP access, same-thread follow-up, and absence of native subagents; it archives its disposable conversations.

## API and migration

Migration `00026_fixed_agent_roles.sql` adopts named roles, preserves models and stored historical prompts, inserts missing roles, and pins project agent IDs. Migration `00027_task_sessions.sql` adds independent-session dispatch tracking without changing task priority/type/stage CHECK values.

- `GET /api/persona` and `GET /api/persona/{id}` include bundled role descriptions, instruction paths, instructions, and skills.
- `PUT /api/persona/{id}` accepts only a model patch, for example `{ "Model": "gpt-5.6-sol" }`.
- Agent creation/deletion and legacy bulk mutations return HTTP 403.
- Project settings cannot replace Conductor or the default Coder identity.
- `POST /api/ai/tasks/{taskId}/session/messages` classifies or queues a follow-up using a message key, previous-job cursor, expected stage, and `auto`, `question`, or explicitly confirmed `work` decision.
