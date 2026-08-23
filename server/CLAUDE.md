# Aycorn Backend — Agent Instructions

Loaded when working in `server/`. Read alongside the root [`CLAUDE.md`](../CLAUDE.md) (cross-cutting rules, the Bulk Actions contract).

---

## Running & Reseeding the App

There is **no live-reload** (no `air`, no watcher). The server is run as a plain:

```
cd server && go run ./cmd/web
```

Implications an agent must account for:

- **Backend changes do not take effect until the server is restarted.** After editing any Go file, the running process is still the old binary. If you (or the user) have a server running, it must be manually killed and re-run. Verifying a new endpoint against a still-running old process returns `404`/`405` — that's a stale binary, not a routing bug.
- **`:8000` gets held by stale processes.** A previous `go run` leaves a `cmd/web`-built binary bound to `:8000`; a fresh run then fails to bind but the old one keeps serving old code (and possibly an old `app.db`). When something behaves like old code, check `lsof -nP -i :8000` before debugging further.
- **Two databases, two `make` targets.** `app.db` is not committed to git — the server auto-creates and migrates it via goose on startup. Its path is controlled by `$AYCORN_DB`; if unset, the binary uses `<os.UserConfigDir()>/aycorn/app.db` (e.g. `~/Library/Application Support/aycorn/app.db` on macOS) — your **personal**, persistent data.
  - `make dev` — no `AYCORN_DB` override, runs against your personal DB. Same DB the installed `aycorn` binary uses.
  - `make dev-test` — sets `AYCORN_DB=./app.db`, runs against a disposable DB at `server/app.db`. Safe to wipe anytime; never touches personal data.
  To reset the test DB to a clean state:
  ```
  cd server
  rm -f app.db
  AYCORN_DB=./app.db go run ./cmd/web   # goose creates all tables automatically
  ```
  (or just `make dev-test`, which sets `AYCORN_DB` for you).
- **`go build ./...` needs the markdown bundle.** `assets/bin/md-convert.cjs` is a gitignored build artifact (like `ui/dist`) that `internal/markdown` embeds, so a fresh clone must run `make build-md-convert` before any build that reaches it. `make build-mcp` does it for you. It is rebuilt from the frontend — see the Markdown Conversion section below.
- **Schema changes go through migration files**, not `schema.sql` directly. See [`server/assets/queries/CLAUDE.md`](../assets/queries/CLAUDE.md) for the full migration workflow.
- **`placeholder.sql` is seed data** for development — load it into the **test** DB only, never the personal one:
  ```
  sqlite3 app.db < assets/queries/placeholder.sql
  ```
- **Backups & restore.** The binary snapshots the DB with SQLite `VACUUM INTO` (see [`cmd/web/backup.go`](cmd/web/backup.go)). On startup, `backupBeforeMigrate` snapshots the DB *before* `goose.Up` whenever the on-disk version is behind the embedded migrations — so an upgrade can never silently lose data; a snapshot failure aborts startup. Snapshots land in a `backups/` folder beside the DB, rotated to the newest `AYCORN_BACKUP_KEEP` (default 10, `0` = keep all). Manual subcommands: `aycorn backup [dest]` and `aycorn restore <src>` (`restore` integrity-checks the snapshot, snapshots the current DB first, then swaps the file in; refuses if an `aycorn` process is detected). `make backup` / `make restore SRC=...` act on your personal DB; `make backup-test` / `make restore-test SRC=...` act on the test DB (`server/backups/`, gitignored).

---

## Backend Architecture (Go)

Strict three-layer separation:

```
Handler → Service → Repository
```

- **Handlers** handle HTTP — parse requests, call services, write responses. No business logic, no SQL.
- **Services** contain all business logic. No SQL here.
- **Repositories** contain all SQL queries. No business logic here.

### Error handling & logging

- Check errors immediately and bubble them up the call stack. No silent failures.
- Log meaningful errors in the service/handler layer.
- Map known service errors to the right HTTP status in the handler (e.g. an invalid-stage-type error → `400`, not `500`). Reserve `500` for genuinely unexpected failures.

---

## Markdown Conversion (`internal/markdown`)

`task.body` is Plate.js document JSON, but the MCP tools speak markdown: `update_task` takes markdown, `read_task` / `search_tasks` return it.

The conversion is **not** implemented in Go. Plate's own serializer is the only thing that knows the app's exact node inventory, so `app/scripts/md-convert.ts` builds a headless editor from the app's plugin kits (`app/src/features/editor/markdown-editor.ts`) and esbuild bundles it into the dependency-free `assets/bin/md-convert.cjs`. `internal/markdown.Converter` extracts that embedded bundle into the user's cache dir and shells out to `node`, one batched JSON request per call.

Consequences:

- **`node` must be on PATH** wherever `aycorn-mcp` runs. A missing node, a crash, or a timeout is a real Go error — never fall back to returning raw Plate JSON as if it were markdown.
- **Conversion is batched.** Process startup dominates, so convert a whole slice in one call rather than looping single conversions.
- **A new node-defining editor plugin needs two edits.** Adding one to `app/src/features/editor/rich-editor.tsx` without adding it to `markdown-editor.ts` means that node silently disappears on conversion.
- **Rebuild the bundle after any editor plugin change**: `make build-md-convert`.

---

## Tests

Plain `go test` — no external assertion library. Run with `make test-server` (it builds the markdown bundle first, since `internal/markdown` embeds it) or `cd server && go test ./...`.

`internal/markdown/converter_test.go` drives the real bundled `md-convert.cjs` under a real `node` rather than mocking the subprocess — the subprocess *is* the thing under test. It `t.Skip`s with a clear message when `node` isn't on PATH, so a machine without node still gets a green suite.

---

## Backend Gotcha: NULL text columns can't scan into `string`

Go's `database/sql` cannot scan a SQL `NULL` into a Go `string` — it errors at runtime (surfaces as a 500 with `converting NULL to string is unsupported`).

Nullable text columns (e.g. `description`) are therefore wrapped in `COALESCE(col, '')` **in the repository's column list**, not handled with `sql.NullString` in the model. Follow that convention for any new query over a nullable text column. Grep existing repos for `COALESCE(` for the pattern.

---

## Database (SQLite)

**Driver:** `modernc.org/sqlite` (registered as `"sqlite"`), a pure-Go transpilation of SQLite — not `mattn/go-sqlite3`. This is deliberate: it needs no C toolchain to build, on any OS. If you ever reach for `mattn/go-sqlite3`'s DSN pragma syntax (`?_foreign_keys=on`) out of habit, use modernc's instead: `?_pragma=foreign_keys(1)` (repeat `_pragma=` per pragma).

**Migrations:** `server/internal/migrations/sql/` — goose applies these on startup. See [`server/assets/queries/CLAUDE.md`](../assets/queries/CLAUDE.md) for how to add migrations.

`server/assets/queries/schema.sql` is a **human-readable reference only** — keep it in sync with migrations when you change the schema, but it is not loaded by the server.

Schema summary:

```sql
workflow  (id, name, description, timeCreated, timeModified)
stage     (id, workflow→workflow.id ON DELETE CASCADE, name, description,
           color, icon, position,
           type CHECK in ('open','todo','doing','done'),
           timeCreated, timeModified)
persona   (id, name, system_prompt, harness, model, allowed_tools JSON array,
           timeCreated, timeModified)
stage_persona (stage_id→stage.id ON DELETE CASCADE,
               persona_id→persona.id ON DELETE CASCADE; one row per stage)
project   (id, name, pinned, workflow→workflow.id, timeCreated, timeModified)
checklist (id, project→project.id, name, timeCreated, timeModified, isDefault)
task      (id, checklist→checklist.id, stage→stage.id ON DELETE RESTRICT,
           name, body, timeCreated, timeModified,
           timePlannedStart, timePlannedEnd, timeCompleted, assignee,
           priority CHECK in ('Urgent','High','Medium','Low'),
           type CHECK in ('Dev','Test','Reminder'))
```

Hierarchy & relationships:
- `workflow → stage` (a workflow owns an ordered list of stages; deleting a workflow cascades to its stages).
- `stage → stage_persona → persona` (a stage has at most one bound persona; deleting either endpoint removes the binding).
- `project → checklist → task`. Tasks belong to a **checklist**, not directly to a project.
- A `project` references one `workflow`. A `task` references one `stage` — that's the task's status. There is no `status` column.

Trigger-enforced invariants (don't reimplement these in app code — rely on them and flag when a change conflicts):
- `timeModified` auto-bumps on every update, and cascades up the chain: `stage → workflow`, `task → checklist → project`, `checklist → project`.
- **Exactly one `open` stage per workflow** — partial unique index `oneOpenStagePerWorkflow`. (This is why bulk stage type/delete exclude `open` via `type <> 'open'`.)
- **One default checklist per project** — trigger-enforced on insert and update.
- `timePlannedEnd` must be set with, and on/after, `timePlannedStart` — trigger raises on violation.
- A task's `timeCompleted` is auto-set when its stage becomes `done` and auto-cleared when it leaves a `done` stage. When seeding a `done`-stage task with an explicit historical `timeCompleted`, that value is preserved (the trigger only fires when `timeCompleted` is NULL on insert).
- `task.stage` FK is `ON DELETE RESTRICT` — a stage with tasks pointing at it cannot be deleted.

Migration notes:
- `body` is a JSON array (Plate.js document format).
- `persona.allowed_tools` is a JSON array with a restrictive `[]` default; harness/model values are validated against curated Go constants rather than SQLite `CHECK` constraints.
- Workflows/stages are fully implemented (custom workflows no longer need a migration). The remaining hardcoded `CHECK` constraints are **`task.priority`, `task.type`, and `stage.type`** — changing those allowed values still requires a **schema migration**. Flag this whenever a feature touches them.
