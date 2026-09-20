-- +goose Up
-- Adopt existing named agents without deleting their saved prompts or history.
-- Runtime definitions come from bundled Markdown; only their models are editable.
ALTER TABLE persona ADD COLUMN builtin_role TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX persona_builtin_role ON persona(builtin_role) WHERE builtin_role <> '';

UPDATE persona SET builtin_role='conductor' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name)) IN ('conductor','orchestrator'));
UPDATE persona SET builtin_role='planner' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name))='planner');
UPDATE persona SET builtin_role='researcher' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name)) IN ('research','researcher'));
UPDATE persona SET builtin_role='coder' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name))='coder');
UPDATE persona SET builtin_role='reviewer' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name))='reviewer');
UPDATE persona SET builtin_role='chatter' WHERE id=(SELECT min(id) FROM persona WHERE lower(trim(name))='chatter');

INSERT INTO persona(name,harness,model,builtin_role)
SELECT name,'codex','gpt-5.6-sol',role FROM (
 SELECT 'Conductor' AS name,'conductor' AS role UNION ALL
 SELECT 'Planner','planner' UNION ALL SELECT 'Research','researcher' UNION ALL
 SELECT 'Coder','coder' UNION ALL SELECT 'Reviewer','reviewer' UNION ALL SELECT 'Chatter','chatter'
) roles WHERE NOT EXISTS(SELECT 1 FROM persona WHERE builtin_role=roles.role);

-- Existing stage choices and prompts remain intact. Agent identities are fixed.
UPDATE conductor_project SET settings=json_set(settings,
 '$.conductorAgentId',(SELECT id FROM persona WHERE builtin_role='conductor'),
 '$.taskAgentId',(SELECT id FROM persona WHERE builtin_role='coder'));

-- +goose Down
-- Forward-only: preserve agent identities, preferences and historical references.
SELECT 1;
