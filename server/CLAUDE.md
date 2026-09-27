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
- **Data directory, two `make` targets.** Nothing in it is committed; the server creates and migrates everything on startup. Its location is `$AYCORN_DATA_DIR`, defaulting to `<os.UserConfigDir()>/aycorn` (e.g. `~/.config/aycorn` on Linux). Inside it:
  - `accounts.db` — accounts, sessions, workspaces, memberships, invites (migrations in `internal/accounts/migrations/`).
  - `workspaces/<id>/app.db` — one database per workspace (migrations in `internal/migrations/sql/`), with that workspace's backups, worktrees, chat workspaces, and environments beside it.
  - A single-user `app.db` from before accounts may also sit there; the server no longer reads it.
  - `make dev` — no override, runs against your personal data directory.
  - `make dev-test` — sets `AYCORN_DATA_DIR=./data`: disposable data at `server/data/` (gitignored). Reset with `rm -rf data` and sign up again.
- **Server environment variables** (beyond host/port): `RESEND_API_KEY` enables invite emails (without it, emails are logged and the inviter gets a link to share); `AYCORN_EMAIL_FROM` sets the sender (must be a Resend-verified domain to reach anyone but the Resend account owner); `AYCORN_PUBLIC_URL` fixes the origin used in invite links (default: the inviter's request origin). Serving other people means binding `AYCORN_HOST` to a Tailscale/LAN address; put TLS in front before exposing it beyond a private network.
- **`go build ./...` needs the markdown bundle.** `assets/bin/md-convert.cjs` is a gitignored build artifact (like `ui/dist`) that `internal/markdown` embeds, so a fresh clone must run `make build-md-convert` before any build that reaches it. `make build-mcp` does it for you. It is rebuilt from the frontend — see the Markdown Conversion section below.
- **Schema changes go through migration files**, not `schema.sql` directly. See [`server/assets/queries/CLAUDE.md`](../assets/queries/CLAUDE.md) for the full migration workflow.
- **`placeholder.sql` is seed data** for development — load it into a **test** workspace only, never a personal one:
  ```
  sqlite3 data/workspaces/<id>/app.db < assets/queries/placeholder.sql
  ```
- **Backups & restore.** The binary snapshots the DB with SQLite `VACUUM INTO` (see [`cmd/web/backup.go`](cmd/web/backup.go)). On startup, `backupBeforeMigrate` snapshots the DB *before* `goose.Up` whenever the on-disk version is behind the embedded migrations — so an upgrade can never silently lose data; a snapshot failure aborts startup. Snapshots land in a `backups/` folder beside the DB, rotated to the newest `AYCORN_BACKUP_KEEP` (default 10, `0` = keep all). Every workspace database and `accounts.db` gets its own `backups/` folder. Manual subcommands act on the single database named by `$AYCORN_DB` (point it at a workspace's `app.db`): `aycorn backup [dest]` and `aycorn restore <src>` (`restore` integrity-checks the snapshot, snapshots the current DB first, then swaps the file in; refuses if an `aycorn` process is detected). `make backup-test WORKSPACE=<id>` / `make restore-test WORKSPACE=<id> SRC=...` act on one workspace of the test data.

---

## Multi-user request flow

See "Accounts & Workspaces" in the root `CLAUDE.md` for the model. In code:

- `cmd/web/server.go` is the front door. It serves health/preview, the account API (`accountHandler.go` → `internal/accounts`), and forwards every other `/api/` request to a workspace: session cookie → `X-Aycorn-Workspace` header → membership check → that workspace's handler.
- `cmd/web/workspace_runtime.go` builds one `app` (repos, services, handlers from `routes.go`) plus its background work (agent worker, job scheduler, project chat, environments, backups) per workspace database. `workspaceRegistry` starts all workspaces at boot and new ones when they're created (`provision`, which also seeds a starter workflow).
- **Adding a workspace feature** works exactly as before: add the handler to `routes.go` and write single-database queries. Never reach into another workspace's database, and derive any on-disk path from the workspace's `dbPath` directory.
- **Adding an account-level feature** (anything about users, organizations, or invites): SQL in `internal/accounts/store.go`, rules in `internal/accounts/service.go`, handler in `cmd/web/accountHandler.go`, route in `server.routes()`.
- The signed-in account is available to any handler via `accountFromContext(r.Context())`.
- **`aycorn-mcp` is still single-database:** it opens `$AYCORN_DB`. Agent runs pass their workspace's database, so they work; an MCP client configured by hand must point `AYCORN_DB` at `data/workspaces/<id>/app.db`.

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

**Migrations:** `server/internal/migrations/sql/` — goose applies these to every workspace database on startup. The accounts database has its own set in `server/internal/accounts/migrations/`, applied through a goose `Provider` (not the package-level goose API, which the workspace migrations use). See [`server/assets/queries/CLAUDE.md`](../assets/queries/CLAUDE.md) for how to add migrations.

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
