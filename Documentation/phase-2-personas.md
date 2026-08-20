# Phase 2 — Personas as Data

Status: Not started

Personas exist as rows you can create and edit. No job queue, no autonomous behavior yet. This phase defines the vocabulary Phase 3 needs.

---

## Ships

- Migration `00008_agent_personas.sql` — `agent_persona` and `stage_persona` tables.
- Backend CRUD: `agentPersonaHandler.go` / `agentPersonaService.go` / `agentPersonaRepo.go`.
- Frontend: persona list + create-empty-then-edit persona page, stage→persona binding UI.

## Why here

Binding personas to **stages** rather than **tickets** resolves three open schema questions at once:

| Open question | Resolution via stage-binding |
|---|---|
| What governs pickup eligibility? | Stage has a persona bound **and** the task carries an explicit opt-in flag — independent of `stage.type`, so no CHECK-constraint table-recreate migration. |
| What happens to `task.assignee`? | Nothing. Stays free-text, human-only. Zero churn in `TaskFacets`, `AssigneeQuery`, or `app/src/features/task/properties/task-assignee.tsx`. |
| Does agent assignment need new task columns? | Not yet — eligibility derives from stage. Add a nullable `agentPersona` override column later only if per-ticket deviation is actually needed. |

## Build

**Schema** — follow `server/internal/migrations/sql/00007_task_relationships.sql` as the template (most recent example of a new entity + relationship in one migration).

```sql
CREATE TABLE agent_persona (
  id             INTEGER PRIMARY KEY,
  name           TEXT NOT NULL,
  systemPrompt   TEXT NOT NULL,
  harness        TEXT NOT NULL,
  model          TEXT NOT NULL,
  allowedTools   TEXT NOT NULL, -- JSON array of MCP tool names, restrictive default
  createdAt      TEXT NOT NULL,
  updatedAt      TEXT NOT NULL
);

CREATE TABLE stage_persona (
  stage    INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE,
  persona  INTEGER NOT NULL REFERENCES agent_persona(id)
);
```

Sync `server/assets/queries/schema.sql` by hand afterward — it isn't loaded by the server and drifts silently if forgotten.

**Backend** — wire `agentPersonaService`/`agentPersonaRepo` into the `app` struct in `server/cmd/web/main.go` and `routes.go`, matching the existing manual-DI pattern. Bulk endpoints return `models.BulkResult` per the root `CLAUDE.md` contract, using `dedupeInts` from `services/util.go`.

**Frontend** — create-empty-then-edit, not a modal form: "New Persona" creates a defaulted row and navigates to it, same pattern as "New Workflow". System-prompt edits save on blur. Semantic color tokens only (no hardcoded Tailwind colors).

**The registry principle** — `allowedTools` makes personas configuration, not hardcoded Go types. Adding a new persona later is a row insert. Make the column `NOT NULL` with a restrictive default: omitting a tools list is a documented anti-pattern that silently grants a persona every tool.

## Done when

- [ ] Migration applied, `schema.sql` synced.
- [ ] Persona CRUD works end to end in the UI, following create-empty-then-edit.
- [ ] Stage→persona binding settable from the workflow/stage editor.
- [ ] At least one persona defined with a deliberately narrow `allowedTools` (this becomes the Phase 3 read-only research persona).
