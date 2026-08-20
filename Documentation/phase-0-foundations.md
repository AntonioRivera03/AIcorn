# Phase 0 — Foundations

Status: Done

No new tables. A weekend of work. Every line item below was verified directly against the current codebase (commit `df492ee`) — re-check if it's been a while, since these files may have moved on.

See `Documentation/ai-architecture.md` for how this phase's decisions constrain everything after it.

---

## Ships

- SQLite opened in WAL mode with a `busy_timeout`, in `cmd/web/main.go`.
- `.gitignore` fixed to cover the WAL sidecar files.
- `POST /api/task/{taskId}/transition` — a compare-and-swap stage-transition endpoint, implemented Handler → Service → Repository per the existing convention.
- The five schema decisions below, written down (not implemented).
- A harness spike, run once, notes kept.

---

## 1. Enable WAL mode + busy_timeout

**Why:** `cmd/web/main.go:159` currently opens the DB as:

```go
db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
```

No `journal_mode` pragma means SQLite is in its default rollback-journal mode. That mode does not safely support two OS processes writing to the same file — and Phase 1 introduces a second process (`cmd/mcp`) that needs to. `backup.go` also opens two more short-lived connections (`backup.go:154`, `backup.go:213`) with the same DSN; those are fine to leave in whatever mode they read (VACUUM INTO reads a consistent snapshot regardless), so only the long-lived connection in `main.go` needs the change.

**Change** (`server/cmd/web/main.go`, the `sql.Open` call at line 159):

```go
db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
```

`modernc.org/sqlite` requires repeating `_pragma=` per pragma (see the gotcha note in `server/CLAUDE.md`) — don't try to comma-join them. `busy_timeout(5000)` makes a writer that arrives mid-write wait up to 5 seconds and retry internally instead of immediately failing with `SQLITE_BUSY`, which matters once there are multiple potential writers (the frontend via `cmd/web`, the Phase 3 worker goroutine in the same process, and `cmd/mcp` in a separate process).

**Verify after the change:**
- `app.db-wal` and `app.db-shm` files appear next to `app.db` after the server runs and writes something.
- `aycorn backup` (or `make backup`) still produces a valid, restorable snapshot — `VACUUM INTO` is documented to work regardless of the source's journal mode, but confirm it empirically once WAL is actually on, don't just trust the docs.
- Existing tests/manual smoke test (create a task, edit it, drag it between stages) still behave identically — WAL is an on-disk format change, not a behavior change, so nothing observable should differ.

## 2. Fix `.gitignore` for WAL sidecar files

**Why:** `.gitignore` (repo root) currently has:

```
app.db
server/dev.db
```

These are exact-filename matches. WAL mode creates `app.db-wal` and `app.db-shm` alongside `app.db`, which this pattern does not cover — they'd show up as untracked files and risk being accidentally `git add -A`'d.

**Change** (`.gitignore`, lines 1 and 4):

```diff
- app.db
+ app.db*
  server/backups/
  server/ui/dist/
- server/dev.db
+ server/dev.db*
```

## 3. The compare-and-swap transition endpoint

**Why:** `PUT /api/task` (`taskHandler.go:129`, `putTask`) decodes a full `models.ChecklistTask` and calls `TaskService.UpdateTask`, which runs (`taskRepo.go:183`, `UpdateTask`):

```go
UPDATE task SET
    name = ?, checklist = ?, timePlannedStart = ?, timePlannedEnd = ?,
    hasTimePlannedStart = ?, hasTimePlannedEnd = ?, timeCompleted = ?,
    assignee = ?, priority = ?, type = ?, stage = ?
WHERE id = ?;
```

This is a whole-row write of everything except `body` (which is deliberately decoupled onto its own endpoint, per the comment at `taskRepo.go:184`) — including `stage`. It has no concurrency check at all: whichever caller's `PUT` lands last wins, silently, on every one of those columns. That's an acceptable trade for a human dragging one kanban card, since there's only one human. It stops being acceptable once a background worker (Phase 3) can also change `stage` — a worker racing a human's in-flight edit would silently clobber whatever else that human's `PUT` was carrying (title, priority, dates), not just the stage field.

This phase adds a narrow, single-purpose endpoint that only ever touches `stage`, and only if `stage` is still what the caller last saw.

**Repository** (`server/internal/models/repos/taskRepo.go`, alongside `UpdateTask`):

```go
// CompareAndSwapStage moves a task from fromStage to toStage only if it is
// still in fromStage. Returns false (no error) if another writer already
// moved it — the caller decides how to handle that, it is not a failure.
func (repo *TaskRepo) CompareAndSwapStage(taskId, fromStage, toStage int) (bool, error) {
    res, err := repo.DB.Exec(
        `UPDATE task SET stage = ? WHERE id = ? AND stage = ?;`,
        toStage, taskId, fromStage,
    )
    if err != nil {
        return false, err
    }
    rowsAffected, err := res.RowsAffected()
    if err != nil {
        return false, err
    }
    return rowsAffected > 0, nil
}
```

**Service** (`server/internal/models/services/taskService.go`, matching the sentinel-error convention already used in `stageService.go` / `taskTypeService.go` / etc.):

```go
var ErrStageConflict = errors.New("task is not currently in the expected stage")

// TransitionStage moves a task between stages with an optimistic-concurrency
// check. Unlike UpdateTask, this never touches any other column.
func (s *TaskService) TransitionStage(taskId, fromStage, toStage int) (bool, error) {
    ok, err := s.TaskRepo.CompareAndSwapStage(taskId, fromStage, toStage)
    if err != nil {
        return false, err
    }
    if !ok {
        return false, ErrStageConflict
    }
    return true, nil
}
```

(Add the `errors` import to `taskService.go` if not already present.)

**Handler** (`server/cmd/web/taskHandler.go`, alongside `putTask`):

```go
func (app *app) transitionTaskStage(w http.ResponseWriter, r *http.Request) {
    defer r.Body.Close()

    taskId, err := strconv.Atoi(r.PathValue("taskId"))
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    body := struct {
        FromStage int `json:"fromStage"`
        ToStage   int `json:"toStage"`
    }{}
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    ok, err := app.taskService.TransitionStage(taskId, body.FromStage, body.ToStage)
    if err != nil {
        respondErr(w, err)
        return
    }

    writeJSON(w, http.StatusOK, ok)
}
```

**Error mapping** (`server/cmd/web/http.go`, add a case to `httpStatusForError`'s switch — it currently maps `ErrWorkflowInUse`/`ErrDuplicateRelationship` to 409, add this alongside them):

```go
case errors.Is(err, services.ErrStageConflict):
    return http.StatusConflict
```

**Route** (`server/cmd/web/routes.go`, immediately after the existing `PUT /api/task` line):

```go
mux.HandleFunc("POST /api/task/{taskId}/transition", app.transitionTaskStage)
```

**Nobody calls this endpoint yet.** That's expected — Phase 3's worker and Phase 1's `move_task_stage` MCP tool are its first two callers, and per `Documentation/ai-architecture.md` §3, both of them call `TaskService.TransitionStage` directly as a Go function, not over HTTP. This endpoint exists for the frontend and any other genuinely-external HTTP caller. Ship it now, unused, so it's not built under time pressure later.

## 4. Harness spike

Run once, by hand, before Phase 4 is anywhere close:

```bash
claude -p "list files in the current directory" --output-format json
echo "exit code: $?"
```

Note down: the shape of the JSON on stdout, what a non-zero exit code looks like for a deliberately-broken prompt, and roughly how long a trivial call takes. This is 30 minutes that will save re-reading Phase 4's harness research from scratch when you get there.

**Spike notes (run 2026-08-19, `claude` CLI 2.1.233):**

- Trivial prompt (`-p "list files in the current directory" --output-format json`): exit code `0`, ~5s wall time (`duration_api_ms` 4988). Top-level JSON keys: `is_error` (bool), `subtype`, `result` (the text answer), `session_id`, `num_turns`, `stop_reason`, `terminal_reason`, `total_cost_usd`, `usage` (token/cache counts), `modelUsage` (per-model cost/token breakdown), `permission_denials` (array). No `error` field on success.
- Deliberately-broken prompt (invalid `--model`): exit code `1`, `is_error: true`, `terminal_reason: "api_error"`, `api_error_status: 404`, `result` holds a human-readable explanation, `total_cost_usd: 0`. Failure is cheap and fast (577ms) — it fails before making a model call.
- Takeaway for Phase 4: exit code alone is a reliable success/failure signal; `is_error` + `result` in the JSON gives the failure reason without parsing stderr.

## 5. Decisions locked here

Not implemented in this phase — decided, so Phase 2/3 don't relitigate them. (Full rationale in the plan artifact; this is the resolution only.)

1. `task.assignee` stays free-text — it's the human field, untouched. No churn in `TaskFacets`, `AssigneeQuery`, or `app/src/features/task/properties/task-assignee.tsx`.
2. Agent personas bind to **stages** (`stage_persona`), not tickets (Phase 2).
3. Pickup eligibility = a `stage_persona` binding exists **and** the task carries an explicit opt-in flag — independent of `stage.type`, so no CHECK-constraint table-recreate migration is ever needed for this.
4. Agent output goes to `agent_run.output` (markdown, Phase 3), never into `task.body` (Plate.js JSON). Don't make an LLM emit Plate's node schema.
5. No app-wide auth is introduced by this plan. See `ai-architecture.md` §6 for what's enforced instead and when to revisit this.

## Done when

- [x] WAL mode + busy_timeout live in `main.go`, verified with real `app.db-wal`/`app.db-shm` files and a working backup/restore cycle.
- [x] `.gitignore` updated, no WAL sidecar files show up under `git status`.
- [x] `CompareAndSwapStage` / `TransitionStage` / `transitionTaskStage` merged, route registered, `ErrStageConflict` mapped to 409 — reachable via `curl` but not called from anywhere else yet.
- [x] Harness spike run once, notes kept somewhere findable (this file's history, or a scratch note).
- [x] The five decisions above are final — if any changed while doing this phase, this file has been edited to match, not just remembered.
