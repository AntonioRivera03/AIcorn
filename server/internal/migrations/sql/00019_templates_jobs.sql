-- +goose Up
CREATE TABLE task_template (
 id INTEGER PRIMARY KEY,
 project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 data TEXT NOT NULL,
 revision INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE scheduled_job (
 id INTEGER PRIMARY KEY,
 project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 template INTEGER NOT NULL REFERENCES task_template(id) ON DELETE CASCADE,
 name TEXT NOT NULL DEFAULT 'New Job',
 agent INTEGER REFERENCES persona(id) ON DELETE SET NULL,
 schedule TEXT NOT NULL DEFAULT '',
 timezone TEXT NOT NULL DEFAULT 'UTC',
 enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 nextRun INTEGER NOT NULL DEFAULT 0,
 revision INTEGER NOT NULL DEFAULT 1,
 lastError TEXT NOT NULL DEFAULT ''
);
CREATE INDEX scheduled_job_due ON scheduled_job(enabled,nextRun);
CREATE TABLE scheduled_job_run (
 id INTEGER PRIMARY KEY,
 job INTEGER REFERENCES scheduled_job(id) ON DELETE SET NULL,
 task INTEGER REFERENCES task(id) ON DELETE SET NULL,
 trigger TEXT NOT NULL CHECK(trigger IN ('manual','schedule')),
 occurrence TEXT NOT NULL,
 createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
 UNIQUE(job,trigger,occurrence)
);

-- +goose Down
DROP TABLE scheduled_job_run;
DROP TABLE scheduled_job;
DROP TABLE task_template;
