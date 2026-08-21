-- +goose Up
ALTER TABLE project ADD COLUMN defaultView TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE project DROP COLUMN defaultView;
