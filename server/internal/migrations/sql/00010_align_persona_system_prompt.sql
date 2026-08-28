-- +goose Up
-- Align persona.system_prompt with task.body: both TEXT DEFAULT '[]' (Plate JSON).
-- Existing personas store plain text or '' — migrate empties to '[]' and preserve
-- valid Plate arrays; invalid JSON falls back to '[]' (editor will show empty doc).
PRAGMA foreign_keys=OFF;

-- Recreate persona with corrected default
CREATE TABLE persona_new (
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

INSERT INTO persona_new (id, name, system_prompt, harness, model, allowed_tools, timeCreated, timeModified)
SELECT
    id,
    name,
    CASE
        WHEN trim(system_prompt) = '' THEN '[]'
        WHEN json_valid(system_prompt) AND json_type(system_prompt) = 'array' THEN system_prompt
        ELSE json_array(json_object('type','p','children',json_array(json_object('text', system_prompt))))
    END,
    harness,
    model,
    allowed_tools,
    timeCreated,
    timeModified
FROM persona;

DROP TRIGGER IF EXISTS persona_timeModified;
DROP TABLE persona;
ALTER TABLE persona_new RENAME TO persona;

-- +goose StatementBegin
CREATE TRIGGER persona_timeModified
AFTER UPDATE ON persona
FOR EACH ROW
BEGIN
    UPDATE persona SET timeModified = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = OLD.id;
END;
-- +goose StatementEnd

PRAGMA foreign_keys=ON;

-- +goose Down
PRAGMA foreign_keys=OFF;

CREATE TABLE persona_old (
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

INSERT INTO persona_old (id, name, system_prompt, harness, model, allowed_tools, timeCreated, timeModified)
SELECT
    id,
    name,
    CASE
        WHEN system_prompt = '[]' THEN ''
        ELSE system_prompt
    END,
    harness,
    model,
    allowed_tools,
    timeCreated,
    timeModified
FROM persona;

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
