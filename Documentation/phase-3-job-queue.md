# Phase 3 — The Job Queue

Status: Not started

Deliberately minimal. Concurrency starts at 1. Proves the full loop — queue, claim, harness invocation, output capture, transition — with the safest possible agent: one that can only read.

---

## Ships

- Migration `00009_agent_jobs.sql` — `agent_job` and `agent_run` tables.
- A single-goroutine ticker worker in `server/cmd/web/main.go`.
- Enqueue-on-transition wiring in `taskService`.
- One read-only "Research" persona proving the loop end to end.
- Minimal UI: a status badge on the task card showing a job is running.

## Why here

This is the riskiest phase to get wrong (it's the first thing writing to the database unattended), so it's scoped to the smallest agent that can prove the mechanism without being able to damage anything: no file-write tools, no repo access.

## Build

**Schema**:

```sql
CREATE TABLE agent_job (
  id          INTEGER PRIMARY KEY,
  task        INTEGER NOT NULL REFERENCES task(id),
  persona     INTEGER NOT NULL REFERENCES persona(id),
  status      TEXT NOT NULL, -- plain text, not CHECK: statuses will grow
  fromStage   INTEGER, toStage INTEGER,
  claimedAt   TEXT, startedAt TEXT, finishedAt TEXT,
  attempts    INTEGER NOT NULL DEFAULT 0,
  error       TEXT,
  createdAt   TEXT NOT NULL
);

CREATE TABLE agent_run (
  id          INTEGER PRIMARY KEY,
  job         INTEGER NOT NULL REFERENCES agent_job(id),
  output      TEXT,   -- markdown, append-only
  summary     TEXT,
  exitCode    INTEGER,
  usageJson   TEXT,
  createdAt   TEXT NOT NULL
);

CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);
```

`status` is deliberately plain text, not a CHECK constraint — a conscious break from the `stage.type` pattern, since job statuses are exactly the kind of value you'll want to add without a full-table-recreate migration.

**Claiming a job** (SQLite idiom — not Postgres `SKIP LOCKED`, which doesn't map to SQLite's single-writer model):

```sql
UPDATE agent_job SET status = 'claimed', claimedAt = ?
WHERE id = (
  SELECT id FROM agent_job
  WHERE status = 'pending' ORDER BY createdAt LIMIT 1
)
RETURNING *;
```

Zero rows affected means nothing to do. On worker startup, reset any `claimed` job older than N minutes back to `pending` — that's the entire crash-recovery story, and it's sufficient at concurrency 1.

**Worker** — a single goroutine with a `time.Ticker` (10s), started from `main.go` alongside the HTTP server goroutine. Polling is simpler than event-driven here, and there's no webhook infrastructure in the app to build the alternative on.

**Trigger** — enqueue a job from inside `taskService`, at the one place stage transitions now flow through (the Phase 0 transition endpoint). Decide explicitly whether bulk kanban drag-drop should also enqueue (recommend: yes, via the same service call) rather than stripping `Stage` from the bulk-update whitelist and breaking multi-select drag.

**First persona** — `allowed_tools` containing only `read_task`, `search_tasks`, `list_projects`; harness run with no file-write tools. It reads the ticket, writes markdown into `agent_run.output`, and transitions the ticket to Review (via the Phase 0 endpoint). If that loop closes end to end, the queue, claim, invocation, output capture, and transition CAS are all validated with zero risk to your filesystem.

## Design notes worth keeping in mind

- Each persona run is a fresh harness process seeded from ticket state, not a long-running conversation — persona drift (context dilution over many turns) is designed out by architecture, not defended against. Same for orchestrator context bloat: the "orchestrator" here is a SQLite table, not an LLM holding state.
- The human moving Review→Coding (rather than an agent auto-advancing) is the "graph interrupt" pattern for human-in-the-loop systems, expressed in a kanban board you already have. It's also the direct fix for the two most-cited multi-agent failure modes: false consensus and infinite handoff loops. If auto-advance is ever wanted, gate it per-stage with an explicit, default-off flag — don't remove the gate by default.

## Done when

- [ ] Migration applied.
- [ ] Worker claims and processes one job at a time, correctly recovers stale `claimed` jobs on restart.
- [ ] End-to-end loop verified: transition into a bound stage → job enqueued → claimed → harness run → `agent_run` written → ticket transitions to Review.
- [ ] Task card shows a visible "agent working" state while a job is in flight.
