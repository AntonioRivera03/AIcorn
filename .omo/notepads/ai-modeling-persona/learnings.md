## 2026-08-22 Plan: AI Modeling & Persona
- 10 tasks total in Ready For Work.
- Wave 1: Task 52 (Persona Data Model) + Task 17 (Assignee Overhaul) in parallel.
- Wave 2: Tasks 53, 54, 55 after 52 (backend APIs + first persona seed).
- Wave 3: Tasks 64-68 after 52/53/54 (frontend UI, use git worktrees to isolate work).
- Stage IDs: Ready For Work=4, Doing=2, Done=3.
- Project tech: React+TS+Tailwind+shadcn app/; Go+SQLite server/.
- Rule: edit-in-place, auto-save, semantic tokens, bulk endpoints use BulkResult.

## 2026-08-22 Task 17: Assignee Overhaul
- Kept `Task.Assignee` as free text; the optional picker collects existing project assignees and optional persona names without changing the persisted field shape.
- `Me` is the stable self-assignment token because Aycorn has no authenticated/current-user identity yet. `Ctrl+Enter` / `Cmd+Enter` and the picker action both assign it.
- Stage persona auto-assignment is client-side at the shared `SelectTaskStage` seam: it applies only for real tasks, only when the task is not assigned to `Me`, and only when the selected stage exposes a persona.
- The picker uses the dedicated `usePersonasQuery` hook against `GET /api/persona`; an empty response remains a valid empty picker, while `Stage.Persona` hydrates stage-bound defaults.
- Touched: `app/src/types/types.ts`, `app/src/features/task/queries/usePersonasQuery.ts`, `app/src/features/stage/select-task-stage.tsx`, `app/src/features/task/properties/task-assignee.tsx`.

## 2026-08-22 Task 52: Persona Data Model
- Migration `00009_personas.sql` creates `persona` and `stage_persona`; `stage_persona.stage_id` is the primary key, so a stage is either unbound or has exactly one persona.
- Both stage and persona foreign keys cascade into `stage_persona`. Deleting either endpoint cannot leave an orphan binding.
- `persona.allowed_tools` is `TEXT NOT NULL DEFAULT '[]'` with a SQLite JSON-array check. Go parsing also maps omitted, blank, `null`, and `[]` to a non-nil empty slice, preserving restrictive access.
- Harness and model deliberately have no SQLite `CHECK`: they remain plain text in storage and are constrained by the typed constants and `IsValidPersonaHarness` / `IsValidPersonaModel` helpers in `internal/models/persona.go`.
- The initial curated vocabulary is harness `claude-code` and model aliases `sonnet`, `opus`, and `haiku`; aliases avoid binding persisted personas to a dated provider model ID.
- `placeholder.sql` contains a development-only read-only researcher persona bound to the Software Project Ready stage. Production persona seeding remains separate from development fixture data.

## 2026-08-22 Task 53: Persona Management API
- Lifecycle endpoints: `GET /api/persona`, `GET /api/persona/{personaId}`, `POST /api/persona`, `PUT /api/persona/{personaId}`, and `DELETE /api/persona/{personaId}`.
- Bulk endpoints: `POST /api/persona/bulk` accepts a JSON array of personas, `PUT /api/persona/bulk` accepts a JSON array of full persona updates, and `POST /api/persona/bulk/delete` accepts a JSON array of persona IDs. Every bulk response uses `{success, failed, skipped}`.
- Bulk create and update execute in one SQLite transaction. Update deduplicates IDs with the last occurrence winning; unknown IDs are intentionally skipped. Delete uses one set-based statement, and the schema's `ON DELETE CASCADE` removes stage bindings in the same atomic operation.
- Harness and model default to `claude-code` and `sonnet` when omitted for create-empty flows, then are validated against the curated constants in the service layer. `allowed_tools` remains a non-nil restrictive empty array when omitted.
- `GET /api/mcp/tools` returns `{name, description}` entries from `internal/mcptools`. The MCP server also builds every registered `mcp.Tool` from that catalog, and an in-memory protocol test verifies the exposed catalog and actual `tools/list` registration stay aligned.

## 2026-08-22 Task 55: First Narrow Research Persona
- The development seed defines `Read-only Researcher` with harness `claude-code`, model `sonnet`, and only `read_task`, `search_tasks`, and `list_projects` in `allowed_tools`.
- The persona is bound to stage 11 (Software Project / Ready), and its seed prompt explicitly limits the run to reading context and producing findings without modifying Aycorn data.
- There is no production seed hook: startup runs goose migrations only. The persona therefore remains in `server/assets/queries/placeholder.sql` and must be loaded into the disposable test DB.

## 2026-08-22 Task 54: Stage-to-Persona Binding API
- `PUT /api/stage/{stageId}/persona` accepts `{"personaId": <id>}` and returns `true`; its insert-select upsert atomically creates or replaces the stage's sole binding and returns `404` if either endpoint does not exist.
- `DELETE /api/stage/{stageId}/persona` clears the binding and returns whether a row was removed. Repeating it is safe and returns `false`.
- `Stage.Persona` is `null` when unbound or a compact `{ID, Name, Harness, Model}` summary when bound; system prompts and tool grants stay on persona detail reads.
- `StageRepo` hydrates the summary with `LEFT JOIN stage_persona` and `LEFT JOIN persona` in the same grouped query used for task counts. `All`, `FindOne`, `ByWorkflow`, `ByWorkflowForProject`, and therefore workflow/project stage responses all receive bindings without per-stage queries.
- Workflow duplication still inserts only fresh stage rows, so copied workflows intentionally remain unbound.

## 2026-08-22 Tasks 67/68: Stage Persona UI Bundle
- The workflow stage row owns a compact searchable persona combobox. It binds and replaces through `PUT /api/stage/{stageId}/persona`, clears through `DELETE`, waits for server confirmation, and exposes a `/personas` link when the catalog is empty.
- Stage persona mutations live in `useStagePersonaMutations.ts`. Success invalidates workflow detail, all workflows, all stages, project details/workflow settings, upcoming tasks, relationship task search, and relationship detail prefixes because each can retain hydrated `Stage` summaries.
- `WorkflowStageChip` is the single cross-surface persona indicator seam. A muted Bot marker is rendered only when `Stage.Persona` exists; its focusable tooltip names the persona without changing the stage tint or stroke classes.
- Workflow rows wrap their controls below the editable stage copy until the `lg` breakpoint. Browser QA at 375, 768, and 1280 px confirmed zero horizontal overflow while preserving the full inline desktop layout.
- The app design contract is now codified in `app/DESIGN.md`, documenting semantic-token-only color, the existing Geist/4px system, stage chips, persona picker states, and keyboard/accessibility constraints.

## 2026-08-22 Tasks 64-66: Persona Navigation, List, and Editor
- Persona UI lives under `app/src/features/persona/`; all persona and MCP-tool TanStack Query hooks are colocated in `features/persona/queries/` and invalidate persona plus workflow caches after mutations.
- `/personas` follows the workflow card-grid pattern with shared drag selection and `BulkActionsToolbarBase`; bulk and single delete confirmations explicitly warn that stage bindings are removed while existing tasks remain unchanged.
- Bound-stage counts are derived from the hydrated `Stages` returned by `GET /api/workflow`, avoiding server changes and extra per-project requests.
- `/personas/$personaId?new=true` autofocuses the editable persona name once, then replaces the URL without the search flag. System prompt persists on blur; harness, model, and allowed tools persist on selection.
- Curated UI vocabularies mirror the backend: harness `claude-code`; models `sonnet`, `opus`, and `haiku`. The tools picker comes from `GET /api/mcp/tools`, and its empty state explicitly means no tool access.
- `PersonaSummary` is now separate from the full `Persona` type so `Stage.Persona` carries only `{ID, Name, Harness, Model}` while persona pages retain prompt, tool, and timestamp fields.
