-- +goose Up
-- Labels such as "Draft" or "Contract", as a JSON array of strings.
ALTER TABLE project_document ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
-- Details read from an uploaded file, shown with the document: a JSON object,
-- e.g. {"email":{"from":…,"to":…,"date":…}} for an imported email.
ALTER TABLE project_document ADD COLUMN details TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE project_document DROP COLUMN details;
ALTER TABLE project_document DROP COLUMN tags;
