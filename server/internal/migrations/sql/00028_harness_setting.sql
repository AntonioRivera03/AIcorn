-- +goose Up
-- The workspace picks one harness (Codex or Claude Code) for every agent.
ALTER TABLE ai_settings ADD COLUMN harness TEXT NOT NULL DEFAULT 'codex';

-- Conductor and Chatter are internal and always run on the default model, so
-- carry Conductor's model over when no default has been chosen yet.
UPDATE ai_settings SET model=COALESCE(
 (SELECT model FROM persona WHERE builtin_role='conductor' AND model<>''), 'gpt-5.6-sol')
WHERE model='';
UPDATE persona SET model='' WHERE builtin_role IN ('conductor','chatter');

-- Planner is retired: Research now writes plans. Move stage bindings and Jobs
-- over; the Planner row stays so run history keeps its reference.
UPDATE stage_persona SET persona_id=(SELECT id FROM persona WHERE builtin_role='researcher')
WHERE persona_id=(SELECT id FROM persona WHERE builtin_role='planner')
 AND EXISTS(SELECT 1 FROM persona WHERE builtin_role='researcher');
UPDATE scheduled_job SET agent=(SELECT id FROM persona WHERE builtin_role='researcher')
WHERE agent=(SELECT id FROM persona WHERE builtin_role='planner')
 AND EXISTS(SELECT 1 FROM persona WHERE builtin_role='researcher');

-- +goose Down
-- Forward-only: model and agent reassignments are not reversible.
SELECT 1;
