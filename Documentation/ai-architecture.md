> Superseded for new AI work by [the explicit-run redesign](ai-integration-redesign-review.md). Retained as historical phase notes; stage bindings and assignee-triggered execution are being retired.

# Aycorn — AI Integration Architecture

An overarching reference for the AI-integration build described in `Documentation/phase-0-foundations.md` through `phase-5-scale.md`. Read this once before starting Phase 0, and re-check it before starting each subsequent phase — it's the thing that keeps Phase 4 from contradicting a decision Phase 1 already made. The phase documents are the authoritative step-by-step; this document is the connective tissue between them: the rules that hold across all of them.

This is guidance to be **loosely followed**, not a spec to satisfy literally. If a phase's real implementation needs to deviate from something here, that's fine — but update this document in the same change, so it keeps being true.

Narrative rationale and the external research behind these decisions live in the published plan artifact (linked from the project's session history); this document only states the resulting architecture, grounded directly in the current codebase.

Last updated: 2026-08-19, against the codebase at commit `df492ee` (schema through migration `00007_task_relationships`). Re-verify file paths and function names below if it's been a while.

---

## 1. Thesis

The MCP server (Phase 1) is not one feature among several — it's the integration seam every other phase reaches through:

- A persona's tool access (Phase 2) is "which MCP tools this persona's harness process is configured with."
- The coding harness (Phase 4) gets ticket context by calling an MCP tool, not a hand-templated prompt.
- The agent trust boundary is the MCP tool surface, not an app-wide auth system Aycorn doesn't otherwise have.

Two things follow from that thesis and hold for the rest of the build:

1. **Business logic lives once, in the Service layer** — every caller (HTTP handler, MCP tool, background worker) is a thin wrapper around the same Service method. See §3.
2. **The human approval gate between workflow stages is the safety mechanism**, not a stopgap to remove later. Don't let a later phase auto-advance a ticket past a stage that has no persona-driven reason to.

## 2. System Map

```
React frontend  ──HTTP──▶  cmd/web
                            Handler → Service → Repository
                            + background job worker (Phase 3, in-process goroutine)
                            (binds 127.0.0.1:8000)
                                 │
                                 │ imports internal/models/services, internal/models/repos
                                 ▼
Chat clients      ──stdio──▶  cmd/mcp                    SQLite (app.db, WAL — see §4)
(Claude Desktop)               (separate OS process,          ▲
                                own *sql.DB handle)             │
                                     │                          │
                                     │ imports the same          │
                                     │ services/ + repos/        │
                                     └────────────────────────────┘

Coding harness (Claude Code, Phase 4) ── invoked by the Phase 3 worker,
    given task context via the cmd/mcp tool surface, writing inside a
    per-job git worktree — never given direct DB access.
```

Two Go binaries share one Go module and one SQLite file. `cmd/web` is the only one that serves HTTP to the frontend and the only one that runs the background worker. `cmd/mcp` is a second, independent OS process (spawned by whatever MCP host is using it — Claude Desktop, Claude Code, etc.) that talks to the *same database* through its own connection, safe because of WAL (§4), and through the *same Go service code*, safe because Go packages aren't tied to the HTTP layer.

## 3. The Shared Service Layer Rule

Every new capability this plan adds — stage transitions, persona CRUD, job claiming, run recording — is implemented **once**, as a method on a `Service` type in `server/internal/models/services/`. Nothing above that layer re-implements SQL, and nothing re-implements the business rule.

There are exactly three call sites for any given Service method, and each new feature should expect to be reachable from the ones that make sense for it:

| Caller | Process | How it calls a Service method |
|---|---|---|
| HTTP handler (`server/cmd/web/*Handler.go`) | `cmd/web` | Direct method call — same process, same struct. |
| MCP tool handler (`server/cmd/mcp/*.go`) | `cmd/mcp` | Direct method call — separate *process*, but the same Go package, imported and constructed the same way `cmd/web/main.go` does it. |
| Background worker goroutine (Phase 3) | `cmd/web` | Direct method call — it's a goroutine inside the same binary as the HTTP handlers, started from `main.go` next to `server.Serve`. |

This is why `cmd/mcp` is worth being a real Go program that imports `internal/models/services` and `internal/models/repos`, rather than an HTTP client of `cmd/web`'s own API: the alternative (MCP tools calling out over `127.0.0.1` HTTP) would work, but it adds a network hop for no isolation benefit once WAL mode makes two direct DB connections safe, and it tempts a future contributor to bypass the Service layer and hit routes directly instead of importing the package. Import the package.

**Rule of thumb when adding anything in Phase 2+:** if you're writing a SQL query outside `internal/models/repos`, or a business rule (validation, defaulting, state-machine logic) outside `internal/models/services`, stop — it belongs in one of those two places, once, regardless of which of the three callers above will use it.

## 4. Data & Concurrency Rules

**WAL mode is a Phase 0 prerequisite for everything after it.** As of this writing, `cmd/web/main.go` opens SQLite with only `?_pragma=foreign_keys(1)` — no `journal_mode` pragma, so SQLite is in its default rollback-journal mode, which does not support two processes writing concurrently. Phase 0 must change this before Phase 1 exists as a second process:

```go
// server/cmd/web/main.go — sql.Open call
db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
```

(`modernc.org/sqlite`'s DSN syntax requires repeating `_pragma=` per pragma — see the gotcha note in `server/CLAUDE.md`.) `busy_timeout` makes a writer that arrives while another write is mid-flight wait up to 5s and retry internally, instead of immediately returning `SQLITE_BUSY` — this matters once there are three potential writers (frontend via `cmd/web`, the Phase 3 worker in the same process, and `cmd/mcp` in a second process).

Two follow-on changes ship in the same Phase 0 commit:

- **`.gitignore`** currently ignores the exact filename `app.db` (and `server/dev.db`). WAL mode creates `app.db-wal` and `app.db-shm` sidecar files that this exact-match pattern does **not** cover. Change both lines to glob patterns: `app.db*` and `server/dev.db*`.
- **Verify `backup.go`'s `VACUUM INTO` still produces a clean single-file snapshot** with WAL enabled (it's documented SQLite behavior that it does, regardless of the source database's journal mode, but confirm empirically once WAL is on rather than trusting docs alone).

**Stage transitions are compare-and-swap, always, once Phase 0 lands.** `TaskService.TransitionStage` (added in Phase 0) is the only sanctioned way to change `task.stage` from Phase 3 onward — the pre-existing `PUT /api/task` / `PUT /api/task/bulk` paths remain as-is for human-driven edits (they're whole-object/whole-field writes with no CAS, which is fine for a human editing one card at a time), but any code written *by this plan* that moves a task between stages — the job worker, an MCP `move_task_stage` tool — must go through the CAS method, never a raw `UPDATE task SET stage = ...`.

**Migrations follow the existing goose convention exactly** — numbered files in `server/internal/migrations/sql/`, `-- +goose Up`/`-- +goose Down` annotations, `-- +goose StatementBegin`/`StatementEnd` around triggers, `schema.sql` updated by hand afterward since it isn't loaded by the server. New tables use `INTEGER PRIMARY KEY AUTOINCREMENT` and `TEXT` timestamp columns defaulting to `strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`, matching every existing table. See `server/internal/migrations/sql/00007_task_relationships.sql` as the template — it's the most recent example of a new entity plus a relationship in one migration.

**`cmd/mcp` should call the same migration bootstrap `cmd/web` does, on its own startup, idempotently.** `goose.Up` is a no-op when already at the latest version, so there's no harm in both binaries calling it — this removes any requirement that `cmd/web` be started first. Extract the DB-path resolution + open + migrate sequence currently living in `cmd/web/main.go` and `cmd/web/backup.go` into a small shared package (e.g. `server/internal/appdb`) that both `cmd/web` and `cmd/mcp` import, rather than duplicating `resolveDBPath`/`backupBeforeMigrate` across two `main` packages. This is exactly the kind of duplication the root `CLAUDE.md` asks agents to flag — flag it by not creating it. Full detail in `phase-1-mcp-server.md`.

## 5. New Schema (cumulative, by end of Phase 4)

| Table | Introduced | Key columns | Notes |
|---|---|---|---|
| `persona` | Phase 2 | id, name, system_prompt, harness, model, allowed_tools (JSON array, `NOT NULL`) | Personas are rows, not Go types — the extensibility mechanism (§6). |
| `stage_persona` | Phase 2 | stage_id (PK, FK→stage, CASCADE), persona_id (FK→persona, CASCADE) | One persona per stage. Pickup eligibility = a binding exists here **and** the task has an explicit opt-in flag — not derived from `stage.type`. |
| `agent_job` | Phase 3 | id, task (FK→task), persona (FK→persona), status (`TEXT`, not CHECK), fromStage, toStage, claimedAt, startedAt, finishedAt, attempts, error, createdAt | `status` deliberately isn't a CHECK constraint — see §4's migration note; job statuses will grow and a CHECK would mean a table-recreate migration every time. |
| `agent_run` | Phase 3 | id, job (FK→agent_job), output (`TEXT`, markdown), summary, exitCode, usageJson, createdAt | Agent output lands here, **never** in `task.body` (which is Plate.js JSON) — don't make an LLM emit Plate's schema. |

`task.assignee` is **not** touched by this plan — it stays the free-text human field it is today (`server/internal/models/repos/taskRepo.go`, `app/src/features/task/properties/task-assignee.tsx`). Pickup eligibility is derived from `stage_persona`, not from anything on `task`.

## 6. Trust Boundary & Extensibility

**No app-wide auth is introduced by this plan.** Aycorn binds to `127.0.0.1` by design (`server/cmd/web/main.go`, `resolveHost`) and has no user/session table today. The MCP stdio transport is a same-machine trust boundary on its own — the host spawns `cmd/mcp` as a child process, so environment-level trust is sufficient. Once Phase 4 lets a harness run LLM-generated code, the risk shifts from "unauthorized access" to "blast radius," and the mitigation is architectural, not a login system: each persona's `allowed_tools` (compiled into that job's MCP config) and per-job git worktree isolation. Revisit this section specifically if the MCP server is ever exposed over Streamable HTTP to a non-local client — that changes the trust boundary and needs real auth (OAuth 2.1 / bearer tokens).

**Personas are configuration, not code.** Adding a new persona (a QA persona, a docs persona) is a row insert into `persona`, never a new Go type or a new switch-case. `allowed_tools` is `NOT NULL` with a restrictive default — a persona with no explicit tool list should have *no* tools, not all of them.

**The coding harness is behind an interface, not hardcoded to one vendor.** OpenCode (the harness originally scoped for this plan) is archived; Claude Code, Crush, Aider, Cline, and Goose all expose roughly the same invocation shape (prompt in, working directory, MCP config, JSON out, exit code). `server/internal/harness.Harness` is the interface; `ClaudeCodeHarness` is the only implementation until there's a real reason for a second one.

```go
type Harness interface {
    Run(ctx context.Context, spec RunSpec) (RunResult, error)
}
```

## 7. Phase Index

| Phase | Document | One-line scope |
|---|---|---|
| 0 | `phase-0-foundations.md` | WAL mode, CAS transition endpoint, gitignore fix, decisions locked |
| 1 | `phase-1-mcp-server.md` | `cmd/mcp` binary, shared `internal/appdb`, read/write MCP tools |
| 2 | `phase-2-personas.md` | `persona` / `stage_persona` tables, persona CRUD UI |
| 3 | `phase-3-job-queue.md` | `agent_job` / `agent_run` tables, single-worker ticker, first read-only persona |
| 4 | `phase-4-coding-harness.md` | `Harness` interface, `ClaudeCodeHarness`, git-worktree-per-job, Coder persona |
| 5 | `phase-5-scale.md` | Concurrency, retries, observability — evidence-driven, no fixed scope |

## 8. Source of Truth Notes

- This document describes an architecture that is **partially not yet built** — §5's schema and §6's `Harness` interface don't exist in the codebase yet as of the "last updated" date above. Everything in §3 and §4 that references *current* code (file paths, function names, the absence of WAL) was verified directly against the repository, not inferred.
- Once Phase 0–1 land, re-verify §4's WAL/pragma claims are actually in `main.go` before trusting this document's description of them as settled.
- For the Go backend's general architecture (unrelated to AI integration), see `Documentation/SAD.md` — this document only covers the additions layered on top of it.
