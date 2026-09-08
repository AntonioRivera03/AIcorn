-- +goose Up
CREATE TABLE environment_installation (
    id INTEGER PRIMARY KEY CHECK(id=1),
    token TEXT NOT NULL
);
INSERT INTO environment_installation VALUES(1, lower(hex(randomblob(8))));

CREATE TABLE environment_settings (
    project INTEGER PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
    settings TEXT NOT NULL CHECK(json_valid(settings)),
    enabledAt INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE task_environment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project INTEGER REFERENCES project(id) ON DELETE SET NULL,
    task INTEGER REFERENCES task(id) ON DELETE SET NULL,
    job INTEGER REFERENCES agent_job(id) ON DELETE SET NULL,
    requestKey TEXT NOT NULL,
    name TEXT NOT NULL,
    repo TEXT NOT NULL,
    branch TEXT NOT NULL,
    sourceCommit TEXT NOT NULL,
    includeChanges INTEGER NOT NULL DEFAULT 0,
    digest TEXT NOT NULL DEFAULT '',
    settings TEXT NOT NULL CHECK(json_valid(settings)),
    state TEXT NOT NULL DEFAULT 'queued',
    desired TEXT NOT NULL DEFAULT 'running' CHECK(desired IN ('running','stopped','deleted')),
    image TEXT NOT NULL DEFAULT '',
    testImage TEXT NOT NULL DEFAULT '',
    testState TEXT NOT NULL DEFAULT 'pending',
    testExitCode INTEGER,
    url TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    logs TEXT NOT NULL DEFAULT '',
    pinned INTEGER NOT NULL DEFAULT 0,
    expiresAt INTEGER NOT NULL,
    createdAt INTEGER NOT NULL DEFAULT (unixepoch()),
    updatedAt INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(project, requestKey)
);
CREATE INDEX task_environment_project ON task_environment(project,id DESC);
CREATE INDEX task_environment_desired ON task_environment(desired,state);
-- Do not cascade away runtime ownership: reconciliation must finish cleanup.
-- +goose StatementBegin
CREATE TRIGGER environment_project_cleanup BEFORE DELETE ON project
BEGIN
    UPDATE task_environment SET desired='deleted',state='deleting',url='',updatedAt=unixepoch() WHERE project=OLD.id AND state<>'deleted';
END;
-- +goose StatementEnd

-- +goose Down
-- Forward-only: dropping ownership records would orphan live cluster resources.
