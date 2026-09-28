-- +goose Up
-- The agent's working and handoff instructions are built into Aycorn now, so
-- projects no longer store them.
UPDATE conductor_project SET settings=json_remove(settings,'$.workingPrompt','$.completionPrompt');

-- A project's Conductor instructions are optional guidance on top of
-- Conductor's fixed instructions. Clear the old default, which only repeated
-- them.
UPDATE conductor_project SET settings=json_set(settings,'$.planningPrompt','')
 WHERE json_extract(settings,'$.planningPrompt')='Inspect the tasks given to Conductor. Start eligible tasks with the appropriate independent agent. Defer tasks with unresolved dependencies or essential missing context, giving a specific reason.';

-- +goose Down
-- Forward-only: the built-in instructions fill these in on read either way.
SELECT 1;
