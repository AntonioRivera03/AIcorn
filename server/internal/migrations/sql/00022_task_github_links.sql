-- +goose Up
CREATE TABLE task_github_link (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('pull_request','branch')),
 label TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL DEFAULT 1,
 createdAt TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(task,url)
);

-- +goose Down
DROP TABLE task_github_link;
