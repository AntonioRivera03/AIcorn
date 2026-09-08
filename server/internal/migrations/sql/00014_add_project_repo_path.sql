-- +goose Up
ALTER TABLE project ADD COLUMN repoPath TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE project DROP COLUMN repoPath;
