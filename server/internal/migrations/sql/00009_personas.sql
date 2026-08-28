-- +goose Up

CREATE TABLE persona (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '',
    harness       TEXT NOT NULL,
    model         TEXT NOT NULL,
    allowed_tools TEXT NOT NULL DEFAULT '[]'
                  CHECK(json_valid(allowed_tools) AND json_type(allowed_tools) = 'array'),
    timeCreated   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    timeModified  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- +goose StatementBegin
CREATE TRIGGER persona_timeModified
AFTER UPDATE ON persona
FOR EACH ROW
BEGIN
    UPDATE persona SET timeModified = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = OLD.id;
END;
-- +goose StatementEnd

CREATE TABLE stage_persona (
    stage_id   INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE,
    persona_id INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE
);

-- +goose Down

DROP TABLE IF EXISTS stage_persona;
DROP TRIGGER IF EXISTS persona_timeModified;
DROP TABLE IF EXISTS persona;
