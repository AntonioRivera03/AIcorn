# MCP task tools

`aycorn-mcp` (`server/cmd/mcp`) serves Aycorn's task data over stdio through the [Model Context Protocol](https://modelcontextprotocol.io), so an AI agent (Claude Desktop, `claude mcp add`, Codex, etc.) can read and write tasks through the same service/repo layer `cmd/web` uses — no HTTP hop, no separate auth. Point an MCP client at `server/bin/aycorn-mcp` (built with `make build-mcp`) with either `AYCORN_DB` set to an explicit database file or `AYCORN_WORKSPACE` set to a workspace id; `node` must be on `PATH` for the markdown conversion described below. With neither variable set, a multi-user data directory refuses to guess and lists its workspaces instead of starting.

This page covers the **external, unscoped catalog** — the tools available to an ordinary MCP client with no run scoping. Aycorn's own AI agents run in narrower, purpose-built sessions layered on the same tool implementations: Chatter (project-scoped, chat-turn-scoped — see [`project-chats.md`](project-chats.md)), Conductor's dispatcher (project-scoped — see [`conductor-mode.md`](conductor-mode.md)), and a task session's own run (locked to its one task). Those sessions see a subset of the tools below, plus a few internal-only tools (`project_context`, `send_to_conductor`, `list_conductor_tasks`, etc.) not documented here.

## Tools

| Tool | Does |
| --- | --- |
| `search_tasks` | Search and filter tasks by query, project, stage, priority, or assignee. Bodies are returned as markdown. |
| `read_task` | Read one task's full details by id, body as markdown. |
| `list_projects` | List all projects. |
| `list_workflow_stages` | List all workflow stages — use this to learn valid stage ids. |
| `create_task` | Create a task on a checklist. The body starts empty; write it with `update_task`. |
| `update_task` | Update a task's name, priority, assignee, body (markdown), checklist, or type. Never moves stage. |
| `move_task_stage` | Move a task to a different stage, with an optimistic-concurrency check (see below). |
| `list_checklists` | List checklists, optionally filtered by project — learn valid checklist ids. |
| `list_task_types` | List task types — learn valid type ids before `create_task`/`update_task`. |
| `list_task_links` | List a task's GitHub pull request and branch references, with their ids and revisions. |
| `add_task_link` | Attach a GitHub pull request or branch URL to a task, with an optional label. |
| `remove_task_link` | Remove a GitHub reference from a task by id and revision. |

`server/internal/mcptools/catalog.go` is the source of truth for names and descriptions; `cmd/mcp/tools_test.go` fails if a registered tool's schema drifts from it.

## Moving a task and changing its type

`move_task_stage` takes the stage the caller currently believes the task is in (`fromStage`) along with the destination (`toStage`) and applies a compare-and-swap: if the task has moved since the caller last read it, the call fails with an actionable tool error telling the model to `read_task` again and retry, rather than silently overwriting a concurrent change.

`update_task`'s `typeId` changes a task's type (for example, moving it into or out of the built-in **Chat** type — see [`ticket-chats.md`](ticket-chats.md)). Only the `type` column is written; the task's body and its `agent_job` history (a chat-type task's message history) are untouched by a type change in either direction.

## GitHub links

`list_task_links`, `add_task_link`, and `remove_task_link` manage `https://github.com/owner/repo/pull/123` and `.../tree/branch` references attached to a task (see [`task-github-links.md`](task-github-links.md) for the human-facing side of the same feature). Only GitHub PR/branch URLs are accepted — no other host, and the link is never fetched or synced with GitHub. Adding a URL that's already attached returns the existing link instead of erroring or duplicating it. Removing a link requires its current `revision` (from `list_task_links`), so a stale caller can't remove a link that changed underneath it. Both writes are checked against task ownership in the same database transaction as the change, so they're refused on a ticket another agent's run currently owns.

## Errors

A tool that references an id an agent got wrong — a missing task, checklist, stage, or task type — returns a clean, specific error such as `task 123 not found` or `stage 42 not found`, never a raw database error like `sql: no rows in result set`. `repos.NotFoundError` (`server/internal/models/repos/agentTaskRepo.go`) is the one place that error is constructed, at whichever query already knows which entity was being looked up; MCP tool code and the agent-write repo functions call it instead of comparing driver error text ad hoc.

A task outside a scoped run's project reads as `task <id> not found` — identical to a task that doesn't exist at all — so a project-scoped or task-scoped session can never use these tools to learn that some other project's task exists.
