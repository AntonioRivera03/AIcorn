# Aycorn — Software Architecture Document

A personal, self-hosted task management app (localhost only) blending Jira-style project tracking with Notion-style flexibility. North star: **flexibility without complexity** — sensible defaults, power-user opt-ins.

Last updated: 2026-08-18 (schema in sync through migration `00007_task_relationships`).

---

## 1. Tech Stack

| Layer | Tech |
|---|---|
| Frontend | React 19 + TypeScript 5.9, Vite (rolldown-vite) |
| Styling | Tailwind CSS 4 + shadcn/ui |
| Client data fetching | TanStack Query 5 |
| Routing | TanStack Router 1 (file-based) |
| Rich text | Plate.js |
| Drag and drop | dnd-kit |
| Forms | React Hook Form |
| Backend | Go 1.25.7, stdlib `net/http` (pattern-based routing, no third-party router) |
| Database | SQLite (`server/app.db`) via `modernc.org/sqlite` (pure-Go driver, no cgo) |
| Migrations | `pressly/goose/v3`, embedded, auto-applied on startup |

---

## 2. Repository Layout

```
app/              # React/TypeScript frontend
server/           # Go backend + SQLite
CLAUDE.md         # cross-cutting agent instructions
Documentation/    # this document
```

---

## 3. Frontend (`app/`)

### Folder structure (`app/src/`)

- `routes/` — file-based route definitions (TanStack Router)
- `features/` — feature-scoped logic/components: `calendar`, `checklists`, `color-picker`, `editor`, `icon-picker`, `projects`, `relationship-types`, `settings`, `stage`, `task`, `task-types`, `upcoming`, `workflows`
- `components/` — shared UI: `ui/` (shadcn primitives), `page/`, `sidebar/`, `project/`, `projects/`, `usage/`
- `contexts/` — `project/`, `task/`
- `queries/` — TanStack Query hooks
- `hooks/`, `lib/`, `types/`, `utils/` — standard support directories

### Pages

| Route | Purpose |
|---|---|
| `/` | Projects dashboard/list |
| `/project/$projectId` | Single project view (checklists/tasks) |
| `/project/settings/$projectId` | Per-project settings |
| `/workflows` | Workflow list |
| `/workflow/$workflowId` | Single workflow (stage/board editor) |
| `/task/$taskId` | Task detail view |
| `/task-links` | Manage task relationship types |
| `/task-types` | Manage task types |
| `/upcoming` | Upcoming tasks view |
| `/settings` | App-wide settings |
| `/usage` | Usage stats/charts |

---

## 4. Backend (`server/`)

### Architecture

Strict three-layer separation:

```
Handler → Service → Repository
```

- **Handlers** — HTTP parsing/response only, no business logic, no SQL.
- **Services** — all business logic, no SQL.
- **Repositories** — all SQL, no business logic.

Route table lives in `server/cmd/web/routes.go`, which also serves the built SPA (`ui/dist`) at `GET /` when present.

### API Endpoints (55 total)

**Dashboard**
- `GET /api/` — dashboard summary: all projects + stat tiles (active/completed/priority/on-time task counts)

**Project**
- `GET /api/project` — list all projects
- `GET /api/project/pinned` — list pinned projects
- `GET /api/project/checklist/{projectId}` — checklists for a project
- `PUT /api/project/bulk/pinned` — bulk set pinned flag
- `POST /api/project/bulk/delete` — bulk delete
- `GET /api/project/{projectId}/settings/workflow` — get project's workflow setting
- `PUT /api/project/{projectId}/settings/workflow` — switch project's workflow
- `GET /api/project/{projectId}/settings/task-types` — get task-type settings
- `PUT /api/project/{projectId}/settings/task-types` — set enabled task types
- `GET /api/project/{projectId}/settings/task-types/enabled` — list enabled task types
- `POST /api/project/{projectId}/settings/task-types/bulk/enable-category` — bulk-enable all types in a category
- `GET /api/project/{projectId}` — get one project
- `PUT /api/project/{projectId}` — update project
- `DELETE /api/project/{projectId}` — delete project
- `POST /api/project` — create project

**Task**
- `GET /api/tasks` — filtered/upcoming task list (search, checklist, type, stage, priority, assignee, project, planned/completed date range)
- `GET /api/tasks/facets` — facet values for filters (filter dropdowns)
- `GET /api/task/{taskId}` — get one task
- `GET /api/task/body/{taskId}` — get task rich-text body
- `PUT /api/task/body/{taskId}` — save task body
- `POST /api/task` — create task
- `PUT /api/task` — update task
- `PUT /api/task/bulk` — bulk update tasks
- `POST /api/task/bulk/delete` — bulk delete tasks
- `DELETE /api/task/{taskId}` — delete task
- `GET /api/task/relationships/{taskId}` — relationships for a task

**Task Relationship Type / Task Relationship**
- `GET /api/task-relationship-type` — list relationship types
- `POST /api/task-relationship-type` — create relationship type
- `PUT /api/task-relationship-type/bulk/behavior` — bulk update behavior
- `POST /api/task-relationship-type/bulk/delete` — bulk delete types
- `PUT /api/task-relationship-type/{id}` — update relationship type
- `PATCH /api/task-relationship-type/{id}/icon` — update icon
- `PATCH /api/task-relationship-type/{id}/names` — update from/to names
- `DELETE /api/task-relationship-type/{id}` — delete type
- `POST /api/task-relationship` — link two tasks
- `POST /api/task-relationship/bulk` — bulk-link tasks
- `DELETE /api/task-relationship/{id}` — unlink

**Checklist**
- `POST /api/checklist/{projectId}` — create checklist under project
- `PUT /api/checklist` — update checklist
- `DELETE /api/checklist/{checklistId}` — delete checklist

**Task Type / Task Type Category**
- `GET /api/task-type` — list task types
- `POST /api/task-type` — create task type
- `PUT /api/task-type/bulk` — bulk update
- `POST /api/task-type/bulk/delete` — bulk delete
- `PUT /api/task-type/{id}` — update
- `DELETE /api/task-type/{id}` — delete
- `GET /api/task-type-category` — list categories
- `POST /api/task-type-category` — create category
- `PUT /api/task-type-category/reorder` — reorder categories
- `PUT /api/task-type-category/{id}` — update category
- `DELETE /api/task-type-category/{id}` — delete category

**Workflow / Stage**
- `GET /api/workflow` — list workflows
- `POST /api/workflow/bulk/delete` — bulk delete
- `POST /api/workflow/bulk/duplicate` — bulk duplicate
- `GET /api/workflow/{workflowId}` — get one workflow
- `POST /api/workflow` — create workflow
- `PUT /api/workflow/{workflowId}` — update workflow
- `DELETE /api/workflow/{workflowId}` — delete workflow
- `PUT /api/workflow/{workflowId}/stages/order` — reorder stages
- `PUT /api/workflow/{workflowId}/stages/move` — bulk move stages
- `POST /api/workflow/{workflowId}/stage` — add stage to workflow
- `GET /api/stage` — list all stages
- `PUT /api/stage/bulk/type` — bulk set stage type
- `PUT /api/stage/bulk/color` — bulk set color
- `PUT /api/stage/bulk/icon` — bulk set icon
- `POST /api/stage/bulk/delete` — bulk delete stages
- `PUT /api/stage/{stageId}` — update stage
- `DELETE /api/stage/{stageId}` — delete stage

Every multi-select surface has a dedicated `/bulk/...` handler returning a standard `BulkResult` — no client-side fan-out over the single-item endpoint (see root `CLAUDE.md`, "Bulk Actions").

---

## 5. Database Schema (SQLite)

Hierarchy: `workflow → stage`, `project → checklist → task`, `project ⇄ task_type` (many-to-many), `task ⇄ task` (via `task_relationship`).

| Table | Key columns | Notes |
|---|---|---|
| `workflow` | id, name, description, timeCreated, timeModified | Owns an ordered list of stages |
| `stage` | id, workflow→workflow FK (CASCADE), name, description, color, icon, position, `type` CHECK(open/todo/doing/done), timestamps | Exactly one `open` stage per workflow (partial unique index `oneOpenStagePerWorkflow`) |
| `project` | id, name, pinned, workflow→workflow FK, timestamps | References one workflow |
| `checklist` | id, project→project FK, name, description, timestamps, isDefault | Exactly one default checklist per project (trigger-enforced) |
| `task` | id, checklist→checklist FK, stage→stage FK (RESTRICT), type→task_type FK, name, body (JSON, Plate.js doc), timeCreated, timeModified, timePlannedStart, timePlannedEnd, hasTimePlannedStart, hasTimePlannedEnd, timeCompleted, assignee, `priority` CHECK(Urgent/High/Medium/Low) | Belongs to a checklist, not directly to a project; `stage` is its status (no separate `status` column) |
| `task_type_category` | id, name, isDefault, sortOrder, timestamps | |
| `task_type` | id, name, description, icon, color, isDefault, category→task_type_category FK (RESTRICT), timestamps | `task.type` was a hardcoded CHECK until migration 00003/00004 promoted it to this FK |
| `project_task_type` | (project, task_type) composite PK, both ON DELETE CASCADE | Which task types are enabled per project |
| `task_relationship_type` | id, fromName, toName, `behavior` CHECK(blocking/subtask/link), icon, color, isSystem, timestamps | Seeded system types: Blocks/Blocked By, Subtask Of/Parent Of, Mentions/Mentioned By |
| `task_relationship` | id, fromTask→task FK (CASCADE), toTask→task FK (CASCADE), relationshipType→task_relationship_type FK (RESTRICT), timestamps | CHECK(fromTask≠toTask), UNIQUE(fromTask, toTask, relationshipType) |

### Trigger-enforced invariants

- `timeModified` auto-bumps on every update and cascades up the chain: `stage → workflow`, `task → checklist → project`, `checklist → project`.
- Exactly one `open` stage per workflow (partial unique index).
- Exactly one default checklist per project (trigger on insert and update).
- `timePlannedEnd` must be set with, and on/after, `timePlannedStart` (trigger raises on violation).
- `task.timeCompleted` auto-sets when its stage becomes `done`, auto-clears when it leaves `done`. Seeded historical `timeCompleted` on insert is preserved (trigger only fires when `timeCompleted` is NULL on insert).
- `task.stage` is `ON DELETE RESTRICT` — a stage with tasks pointing at it cannot be deleted. Same pattern for `task_type.category` and `task_relationship.relationshipType`.

### Remaining hardcoded CHECK constraints

`task.priority` and `stage.type` still use hardcoded `CHECK` constraints — changing their allowed values requires a table-recreate migration (see `server/assets/queries/CLAUDE.md`). `task.type` is no longer one of these; it moved to a proper FK into `task_type` in migration `00003`/`00004`.

---

## 6. Source of Truth Notes

- The live schema is controlled by goose migrations in `server/internal/migrations/sql/`, not `server/assets/queries/schema.sql` directly — `schema.sql` is a human-readable reference kept in sync manually.
- This document is a point-in-time snapshot. Re-derive from `server/assets/queries/schema.sql`, `server/cmd/web/routes.go`, and `app/src/routes/` if it's been a while — don't treat this as authoritative for current state without checking.
