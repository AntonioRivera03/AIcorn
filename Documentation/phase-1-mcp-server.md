# Phase 1 — The MCP Server

Status: Not started

Depends on Phase 0 (WAL mode must be live before a second process opens the DB). Zero new infrastructure beyond two new Go packages. This is the first phase that's useful entirely on its own.

See `Documentation/ai-architecture.md` §3–4 for the architectural reasoning this phase implements: one shared Service layer, reached by three different callers, safe under WAL.

---

## Ships

- `server/internal/appdb/` — DB path resolution, opening, and migration bootstrap, extracted out of `cmd/web` so `cmd/mcp` can reuse it instead of duplicating it.
- `server/cmd/mcp/` — a new binary, MCP over stdio, importing the existing `internal/models/services` and `internal/models/repos` packages directly.
- Read tools: `search_tasks`, `read_task`, `list_projects`, `list_workflow_stages`.
- Write tools: `create_task`, `update_task`, `move_task_stage`.

---

## 1. Extract `internal/appdb`

**Why:** `cmd/mcp` needs the same DB-path resolution and startup-migration logic `cmd/web/main.go` already has (`resolveDBPath`, the `backupBeforeMigrate` → `goose.Up` sequence). Those currently live as unexported functions in `package main` under `cmd/web` — a second `main` package can't import them. Rather than copy-pasting them into `cmd/mcp` (the exact duplication the root `CLAUDE.md` asks agents to flag), move them into a small internal package both binaries import.

**What moves, from `cmd/web/main.go`:**

| Old | New |
|---|---|
| `resolveDBPath()` (main.go:37) | `appdb.ResolveDBPath()` |

**What moves, from `cmd/web/backup.go`** (all of it is either a dependency of `backupBeforeMigrate` or `backupBeforeMigrate` itself — none of it is CLI-specific):

| Old | New |
|---|---|
| `resolveBackupDir(dbPath)` | `appdb.ResolveBackupDir(dbPath)` |
| `timestampedName(suffix)` | `appdb.TimestampedName(suffix)` |
| `backupKeep()` | `appdb.BackupKeep()` |
| `snapshot(db, dest)` | `appdb.Snapshot(db, dest)` |
| `rotateBackups(dir, keep)` | `appdb.RotateBackups(dir, keep)` |
| `latestMigrationVersion()` | `appdb.LatestMigrationVersion()` |
| `backupBeforeMigrate(db, dbPath)` | `appdb.BackupBeforeMigrate(db, dbPath)` |

**What stays in `cmd/web/backup.go`** — the CLI-only surface (`aycorn backup`, `aycorn restore`), now calling the exported `appdb` functions instead of local ones: `runBackup`, `runRestore`, `integrityCheck`, `aycornRunning`, `copyFile`.

**New file, `server/internal/appdb/appdb.go`:**

```go
package appdb

import (
    "database/sql"
    "fmt"
    "os"
    "path/filepath"
    "strconv"

    "github.com/pressly/goose/v3"
    "github.com/waseem-polus/aycorn/server/internal/migrations"
)

// ResolveDBPath returns the SQLite file path — moved from cmd/web/main.go,
// logic unchanged. See that history for the precedence rules (AYCORN_DB env
// var, then <UserConfigDir>/aycorn/app.db).
func ResolveDBPath() (string, error) {
    if p := os.Getenv("AYCORN_DB"); p != "" {
        return p, nil
    }
    cfgDir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    dir := filepath.Join(cfgDir, "aycorn")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return "", err
    }
    return filepath.Join(dir, "app.db"), nil
}

// Open opens the DB with the app's standard pragmas: foreign keys on, WAL
// journal mode (required so cmd/web and cmd/mcp can hold connections to the
// same file at once — see Documentation/ai-architecture.md §4), and a busy
// timeout so a writer that arrives mid-write retries instead of failing.
func Open(dbPath string) (*sql.DB, error) {
    return sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
}

// Migrate snapshots the DB (if it exists and has pending migrations) and
// applies any pending goose migrations. Idempotent — safe to call from
// multiple binaries/processes on startup; goose.Up no-ops once at the latest
// version, so cmd/mcp does not need cmd/web to have run first.
func Migrate(db *sql.DB, dbPath string) error {
    dbExisted := false
    if fi, err := os.Stat(dbPath); err == nil && fi.Size() > 0 {
        dbExisted = true
    }
    goose.SetBaseFS(migrations.Files)
    if err := goose.SetDialect("sqlite3"); err != nil {
        return err
    }
    if dbExisted {
        if err := BackupBeforeMigrate(db, dbPath); err != nil {
            return err
        }
    }
    return goose.Up(db, "sql")
}
```

`server/internal/appdb/backup.go` gets the seven functions from the table above, verbatim from today's `cmd/web/backup.go`, capitalized and with their doc comments intact.

**`cmd/web/main.go` changes** — replace the manual `resolveDBPath` / `sql.Open` / `goose.SetBaseFS` / `dbExisted` / `backupBeforeMigrate` / `goose.Up` block (main.go:37-50, 146-178) with:

```go
dbPath, err := appdb.ResolveDBPath()
if err != nil {
    log.Fatal(err)
}
log.Printf("Using database at %s", dbPath)

db, err := appdb.Open(dbPath)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

if err := appdb.Migrate(db, dbPath); err != nil {
    log.Fatal(err)
}
```

This also folds the Phase 0 WAL-pragma change into `appdb.Open` — if Phase 0 edited the inline `sql.Open` call directly, this phase moves that same DSN string into `appdb.Open` and deletes the inline version.

**`cmd/web/backup.go` changes** — delete the seven moved functions; update `runBackup`/`runRestore`/`integrityCheck` to call `appdb.ResolveDBPath()`, `appdb.Snapshot()`, `appdb.ResolveBackupDir()`, `appdb.TimestampedName()`, `appdb.RotateBackups()`, `appdb.BackupKeep()` instead.

## 2. `cmd/mcp` — scaffold

```
go get github.com/modelcontextprotocol/go-sdk
```

**`server/cmd/mcp/main.go`:**

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/waseem-polus/aycorn/server/internal/appdb"
    "github.com/waseem-polus/aycorn/server/internal/models/repos"
    "github.com/waseem-polus/aycorn/server/internal/models/services"
    _ "modernc.org/sqlite"
)

func main() {
    // stdout is the JSON-RPC channel for stdio transport — a single stray
    // write there corrupts the whole stream. log defaults to stderr already;
    // this line makes that explicit so it can't regress silently.
    log.SetOutput(os.Stderr)

    dbPath, err := appdb.ResolveDBPath()
    if err != nil {
        log.Fatal(err)
    }
    db, err := appdb.Open(dbPath)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    if err := appdb.Migrate(db, dbPath); err != nil {
        log.Fatal(err)
    }

    // Same wiring pattern as cmd/web/main.go's app struct — construct the
    // repos and services this binary's tools need. Not all of cmd/web's
    // services are needed here; add more as tools need them.
    taskRepo := &repos.TaskRepo{DB: db}
    taskTypeRepo := &repos.TaskTypeRepo{DB: db}
    projectRepo := &repos.ProjectRepo{DB: db}
    stageRepo := &repos.StageRepo{DB: db}

    toolset := &toolset{
        taskService:    &services.TaskService{TaskRepo: taskRepo, TaskTypeRepo: taskTypeRepo},
        projectService: &services.ProjectService{ProjectRepo: projectRepo, TaskRepo: taskRepo},
        stageService:   &services.StageService{StageRepo: stageRepo},
    }

    srv, err := mcp.NewServer(&mcp.Implementation{Name: "aycorn-mcp", Version: "0.1.0"}, nil)
    if err != nil {
        log.Fatal(err)
    }
    toolset.register(srv)

    if err := srv.Run(context.Background(), mcp.NewStdioTransport()); err != nil {
        log.Fatal(err)
    }
}
```

> The exact `mcp.NewServer` / `AddTool` / transport constructor signatures depend on the go-sdk version you land on — treat the shape above as representative, not copy-paste-exact, and check the installed version's godoc (`go doc github.com/modelcontextprotocol/go-sdk/mcp`) once it's vendored. The SDK's Go API surface is less stable than its Python/TypeScript counterparts as of this writing.

## 3. Tools

`server/cmd/mcp/tools.go` — one file, one `toolset` struct holding the services each tool needs, one method per tool, registered from `register(srv)`.

**`search_tasks`** — wraps `TaskService.GetAllTasks`. `repos.TaskFilters`'s `StageQuery`/`ChecklistQuery` fields are `[]string` but are compared against integer columns (`t.stage IN (...)`, `taskRepo.go:518`) — they're stringified IDs, not names. `AllTasks` has no `LIMIT`/pagination at the SQL level (`taskRepo.go:466-596`); for this app's expected data volume, truncating in Go after the fetch is a deliberate simplification, not an oversight — revisit only if a project ever holds thousands of tasks.

```go
type SearchTasksInput struct {
    Query      string   `json:"query,omitempty" jsonschema:"free-text search over the task name"`
    ProjectIDs []int    `json:"projectIds,omitempty"`
    StageIDs   []int    `json:"stageIds,omitempty" jsonschema:"stage ids — call list_workflow_stages for valid ids"`
    Priorities []string `json:"priorities,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
    Assignees  []string `json:"assignees,omitempty"`
    Limit      int      `json:"limit,omitempty" jsonschema:"default 25, max 100"`
}

func (t *toolset) searchTasks(ctx context.Context, req *mcp.CallToolRequest, in SearchTasksInput) (*mcp.CallToolResult, []models.TaskWithProject, error) {
    filters := &repos.TaskFilters{
        SearchQuery:    in.Query,
        ProjectIDQuery: in.ProjectIDs,
        PriorityQuery:  in.Priorities,
        AssigneeQuery:  in.Assignees,
    }
    for _, id := range in.StageIDs {
        filters.StageQuery = append(filters.StageQuery, strconv.Itoa(id))
    }

    tasks, err := t.taskService.GetAllTasks(filters)
    if err != nil {
        return nil, nil, err
    }

    limit := in.Limit
    if limit <= 0 {
        limit = 25
    }
    if limit > 100 {
        limit = 100
    }
    if len(tasks) > limit {
        tasks = tasks[:limit]
    }
    return nil, tasks, nil
}
```

**`read_task`** — wraps `TaskService.GetTask(taskId)`. Input: `{ taskId int }`. Output: `*models.TaskWithProject`.

**`list_projects`** — wraps `ProjectService.GetAllProjects()` (`projectHandler.go:13`). No input.

**`list_workflow_stages`** — wraps `StageService.GetAllStages()` (`stageHandler.go:13`, same call `GET /api/stage` uses). No input. This is how an agent learns valid stage ids for `search_tasks`/`move_task_stage` — call it first, or cache its result for the session.

**`create_task`** — wraps `TaskService.CreateChecklistTask(&task)`. Input mirrors the fields `POST /api/task` accepts (`taskHandler.go:111`, `postTask`): checklist id, name, priority, type id, assignee, planned dates. Leave `Body` empty — bodies are written through the task's own workflow, not at creation.

**`update_task`** — deliberately narrower than `PUT /api/task`. `TaskService.UpdateTask` writes every column in one shot including `stage` (`taskRepo.go:183`); if this tool round-tripped a caller-supplied `stage` verbatim, an agent working from stale context could silently move a ticket sideways of `move_task_stage`'s CAS check. Instead:

```go
type UpdateTaskInput struct {
    TaskID   int     `json:"taskId"`
    Name     *string `json:"name,omitempty"`
    Priority *string `json:"priority,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
    Assignee *string `json:"assignee,omitempty"`
}

func (t *toolset) updateTask(ctx context.Context, req *mcp.CallToolRequest, in UpdateTaskInput) (*mcp.CallToolResult, bool, error) {
    current, err := t.taskService.GetTask(in.TaskID)
    if err != nil {
        return nil, false, err
    }
    if in.Name != nil {
        current.Name = *in.Name
    }
    if in.Priority != nil {
        current.Priority = *in.Priority
    }
    if in.Assignee != nil {
        current.Assignee = *in.Assignee
    }
    // current.Stage is whatever it already was — never set from this tool.
    ok, err := t.taskService.UpdateTask(&current.ChecklistTask)
    return nil, ok, err
}
```

Read-modify-write against the current row, only overwriting the fields the caller actually passed, `stage` always round-tripped unchanged. This is the same pattern the frontend already uses (`app/src/queries/useTaskMutation.ts` sends `{ ...task, changedField: newValue }`), just expressed server-side instead of trusting the caller to do it.

**`move_task_stage`** — the only tool allowed to change `stage`, and it goes through Phase 0's CAS method directly (per `ai-architecture.md` §3 — a Go call, not an HTTP request to `cmd/web`):

```go
type MoveTaskStageInput struct {
    TaskID    int `json:"taskId"`
    FromStage int `json:"fromStage" jsonschema:"the stage id the caller currently believes the task is in"`
    ToStage   int `json:"toStage"`
}

func (t *toolset) moveTaskStage(ctx context.Context, req *mcp.CallToolRequest, in MoveTaskStageInput) (*mcp.CallToolResult, bool, error) {
    ok, err := t.taskService.TransitionStage(in.TaskID, in.FromStage, in.ToStage)
    if errors.Is(err, services.ErrStageConflict) {
        // Return this as a tool error with an actionable message, not a bare
        // 409 — the calling model should re-read the task and retry, not give up.
        return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{
            &mcp.TextContent{Text: "task is no longer in fromStage — call read_task again and retry with the current stage"},
        }}, false, nil
    }
    return nil, ok, err
}
```

## 4. Registration & logging

Register every tool with an explicit `Description` and a schema derived from the input struct's `json`/`jsonschema` tags — per the MCP research, a tool with a vague or missing description forces the calling client to either prompt for confirmation on every call or auto-approve blindly; neither is good. One atomic operation per tool, no combined "manage_task" tool that branches on an action field — that's the pattern the MCP tooling research flags as causing agents to sequence calls incorrectly.

## Done when

- [ ] `internal/appdb` exists; `cmd/web` builds and runs identically to before (same DB path, same backup/restore CLI behavior) using it.
- [ ] `cmd/mcp` builds, runs standalone, connects to the same `app.db` while `cmd/web` is also running, with no `SQLITE_BUSY` errors under normal use.
- [ ] All four read tools return real data end to end.
- [ ] `create_task` / `update_task` / `move_task_stage` work; `update_task` verified to never change `stage`; `move_task_stage` verified to return the conflict message (not a crash) when raced.
- [ ] Registered in a real MCP host (Claude Desktop or `claude mcp add`) pointing at the **built binary path**, not `go run` — MCP hosts spawn the executable directly.
- [ ] Used for real, in real chat sessions, for about a week before Phase 2 starts.
