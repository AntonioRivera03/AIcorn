-- +goose Up
-- Add persona.agent column for opencode agent binding (11 catalog values, empty = none).
-- No CHECK constraint; validation lives in Go.
ALTER TABLE persona ADD COLUMN agent TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite cannot DROP COLUMN easily; recreate table without agent column.
PRAGMA foreign_keys=OFF;

CREATE TABLE persona_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '[]',
    harness       TEXT NOT NULL,
    model         TEXT NOT NULL,
    allowed_tools TEXT NOT NULL DEFAULT '[]'
                  CHECK(json_valid(allowed_tools) AND json_type(allowed_tools) = 'array'),
    timeCreated   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    timeModified  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO persona_old (id, name, system_prompt, harness, model, allowed_tools, timeCreated, timeModified)
SELECT id, name, system_prompt, harness, model, allowed_tools, timeCreated, timeModified FROM persona;

DROP TRIGGER IF EXISTS persona_timeModified;
DROP TABLE persona;
ALTER TABLE persona_old RENAME TO persona;

-- +goose StatementBegin
CREATE TRIGGER persona_timeModified
AFTER UPDATE ON persona
FOR EACH ROW
BEGIN
    UPDATE persona SET timeModified = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = OLD.id;
END;
-- +goose StatementEnd

PRAGMA foreign_keys=ON;
