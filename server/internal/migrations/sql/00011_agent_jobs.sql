-- +goose Up
CREATE TABLE agent_job (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task       INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
    persona    INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE,
    status     TEXT NOT NULL,
    fromStage  INTEGER,
    toStage    INTEGER,
    claimedAt  TEXT,
    startedAt  TEXT,
    finishedAt TEXT,
    attempts   INTEGER NOT NULL DEFAULT 0,
    error      TEXT,
    createdAt  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE agent_run (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    job       INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE,
    output    TEXT,
    summary   TEXT,
    exitCode  INTEGER,
    usageJson TEXT,
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_job_status;
DROP TABLE IF EXISTS agent_run;
DROP TABLE IF EXISTS agent_job;
