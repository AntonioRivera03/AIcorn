-- +goose Up
CREATE TABLE project_document (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '[{"type":"p","children":[{"text":""}]}]',
    revision INTEGER NOT NULL DEFAULT 1,
    timeCreated TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    timeModified TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX project_document_project ON project_document(project, id);

-- +goose Down
DROP TABLE project_document;
