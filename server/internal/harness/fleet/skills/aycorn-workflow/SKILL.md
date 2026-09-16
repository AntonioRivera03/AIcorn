---
name: aycorn-workflow
description: Use Aycorn MCP for project task selection and independent task execution. Applies to Conductor dispatch and task agent sessions.
---

## Project dispatcher

Conductor uses `project_context`, `list_conductor_tasks`, `read_project_document`, `start_conductor_task` and `defer_conductor_task`. It starts only waiting tasks explicitly handed to Conductor. Pass the project ID, task ID and one supported role; code resolves prompts, models and permissions. The start tool queues a separate Codex session and moves the task to the configured working stage atomically. Do not spawn subagents or wait for their results. Report queued work as queued.

## Independent task session

Read the assigned ticket with `read_task` and relevant project context before working. MCP reads are scoped to this project; mutations, if exposed, are restricted to the assigned active task. Document reads return written notes and attachment metadata, not binary attachment contents. Follow repository instructions and use the assigned workspace.

Pursue the task objective and current request through completion, including appropriate validation. The application injects fixed role instructions and the current turn mode. In question mode, explain existing work without making changes or continuing the task. In work mode, complete the requested work autonomously and return the required handoff format. Never infer stage IDs from names or move tasks yourself. Server code owns transitions, resumes the existing conversation, and moves successful work to the configured human review stage. Failures, cancellations and blockers must remain outside review. Only a human marks work Done.

If MCP or essential context is unavailable, report the specific blocker. Do not bypass tool restrictions through database access, another connection, or native delegation. Report only changes, sources and tests you actually verified.
