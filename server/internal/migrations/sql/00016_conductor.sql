-- +goose Up
CREATE TABLE conductor_project (
 project INTEGER PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
 enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 settings TEXT NOT NULL
);
CREATE TABLE conductor_task (
 task INTEGER PRIMARY KEY REFERENCES task(id) ON DELETE CASCADE,
 project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 state TEXT NOT NULL DEFAULT 'waiting' CHECK(state IN ('waiting','planning','needs_context','queued','working','completed','failed','held')),
 job INTEGER REFERENCES agent_job(id) ON DELETE SET NULL,
 expectedStage INTEGER NOT NULL,
 message TEXT NOT NULL DEFAULT '',
 updatedAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX conductor_tasks_by_project ON conductor_task(project, state);

-- +goose Down
DROP TABLE conductor_task;
DROP TABLE conductor_project;
