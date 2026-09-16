# Jobs and task templates

Project Settings → **Jobs & templates** holds reusable task blueprints and their schedules. Creating a template immediately adds an editable empty template. Fields save when you leave them; Escape discards an unsaved field edit. A template contains its display name, task title, Markdown body, checklist, default workflow stage, task type, priority, assignee, and additional agent instructions.

**Create task** stamps a normal task using the template's current content and properties, without starting AI. Subsequent template edits do not change existing tasks. **Convert to job** creates a separate Job that references the template. Choose a custom Codex agent to supply the coder's model and instructions. The project Conductor agent remains the orchestrator; if one is not selected, the Job's agent supplies both roles. Configure three distinct planning, working, and review stages in the project's Conductor settings. Its repository option and phase prompts also apply to Jobs.

**Run now** stamps and queues one task immediately. It does not require a schedule or an enabled board Conductor. Jobs use the same durable planning → working → human review pipeline as delegated board tasks. Conductor handles ticket ownership and stage transitions; the selected task agent supplies the coder's instructions/model. If the result needs more context, add it to the task and use **Recheck**. That preserves the Job's frozen fleet and independent run mode even while the board Conductor remains paused. A completed task retains normal AI history, native Codex session links, file changes, its isolated branch, and preview/merge actions. Completion hands off for human review rather than marking the task Done.

## Scheduling policy

- Schedules use five-field cron (`minute hour day month weekday`) and an IANA timezone such as `America/Chicago`. `0 9 * * 1` means Monday at 09:00 in that timezone. The timezone database is bundled; daylight-saving changes affect the corresponding UTC time.
- Schedules start paused. A blank cron expression is valid for manual-only Jobs. Enabling a schedule checks the agent, local Codex setup, template references, and workflow stages. Existing schedules use the latest template and settings when each occurrence is prepared; the resulting run freezes those inputs.
- The server checks due schedules on startup and every 15 seconds, independently of the harness worker. Aycorn must be running; this is not an OS scheduler. Scheduling is disabled in application previews.
- A due occurrence atomically creates the task, its independent Conductor state, queue request, and occurrence record. A transaction either commits all of them or none of them. The persisted occurrence identity prevents duplicate tasks across retries/restarts; manual requests also carry an idempotency key.
- After downtime, one overdue occurrence runs, and the next time is calculated strictly after the current time. Missed occurrences do not accumulate into a backlog.
- Only one unfinished run of a given Job can be pending, planning, queued, working, or canceling. Scheduled overlaps are skipped and advance to the next cadence. The Job shows “Skipped overlapping run.” Manual overlaps return a conflict. Other Jobs may queue normally; the existing worker runs one harness session at a time.
- A failed setup creates no task, records an actionable error on the Job, and advances to the next cadence. Cancellation during server shutdown leaves the occurrence due. If the server restarts during a model run, the normal worker marks it interrupted; inspect its task and explicitly recheck instead of silently replaying work.
- Pausing a schedule stops future automatic firings and lets an active run finish. Run now remains available. Pausing the board Conductor does not pause independent Jobs.

The Job's recent history links to its last 50 tasks. Deleting a completed Job preserves its tasks and their AI results. An active Job cannot be deleted; release or cancel its task and wait for execution to stop first. A template referenced by a Job cannot be deleted until those Jobs are removed. Deleting the enclosing project removes its scheduling configuration through the normal project deletion transaction.

## API and storage

Endpoints are scoped to `/api/project/{projectId}/automation`:

| Resource | Operations |
| --- | --- |
| `/templates` | GET list, POST create empty |
| `/templates/{id}` | GET, PUT full template with revision, DELETE |
| `/templates/{id}/instantiate` | POST → `{taskId}` |
| `/templates/{id}/job` | POST → new paused Job |
| `/jobs` | GET list |
| `/jobs/{id}` | GET, PUT full Job with revision, DELETE |
| `/jobs/{id}/run` | POST `{key: "unique-click-id"}` → `{taskId}` |
| `/jobs/{id}/runs` | GET recent history |

Revision checks reject stale edits with HTTP 409. Cross-project references are rejected. `scheduled_job` is the user-facing recurring Job; `agent_job` remains the internal durable queue. Migration 00019 adds `task_template`, `scheduled_job`, and `scheduled_job_run`; no existing stage or priority CHECK values change. Cron parsing and timezone calculations use `robfig/cron/v3`.

## Verification

The Jobs service tests use a migrated SQLite database and the real queue/worker. They cover template snapshots and scope, concurrent manual idempotency, overlap handling, missed cadence, failed setup, timezone/DST calculations, automatic loop startup/shutdown, and both manual and scheduled planning-to-review runs while the board toggle is off. Repository runs use temporary Git repositories and verify retained branches and diffs. Recheck tests verify that missing-context recovery preserves the Job agent and prompts. HTTP tests cover CRUD, malformed requests, revision conflicts, queue creation, and preview execution restrictions.

Browser verification uses a disposable database: create and rapidly edit a template, instantiate its body/settings, convert it to a Job, select an agent, edit cron/timezone, display validation errors, and verify the delete confirmation. Actual model transport and subagent delegation are separately covered by the installed-Codex opt-in tests documented in `local-harnesses.md`.
