-- +goose Up
-- Preserve names/prompts and job history. Existing TEXT columns have no CHECK
-- constraints; new writes validate Codex/OpenAI in the service layer.
UPDATE persona SET harness='codex', model=CASE
 WHEN model LIKE 'gpt-%' OR model GLOB 'o[0-9]*' THEN model
 WHEN model LIKE 'openai/gpt-%' THEN substr(model,8)
 WHEN model LIKE 'opencode-go/gpt-%' THEN substr(model,13)
 ELSE 'gpt-5.6-sol' END;
UPDATE ai_settings SET executable='', model=CASE
 WHEN model LIKE 'gpt-%' OR model GLOB 'o[0-9]*' THEN model
 WHEN model LIKE 'openai/gpt-%' THEN substr(model,8)
 WHEN model LIKE 'opencode-go/gpt-%' THEN substr(model,13)
 ELSE 'gpt-5.6-sol' END;
UPDATE agent_job SET status='interrupted', error='The AI engine changed to Codex. Start a new run or recheck this Conductor task.', finishedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE status IN ('pending','claimed','running','canceling');
UPDATE conductor_project SET enabled=0, settings=json_set(json_remove(settings,'$.plannerModel','$.workerModels'),'$.enabled',json('false'),'$.conductorAgentId',0,'$.taskAgentId',0);
UPDATE conductor_task SET state='held',message='Select your Codex agents in project settings, then recheck this task.' WHERE state IN ('waiting','planning','queued','working');

-- +goose Down
-- Forward-only: provider migration preserves historical request snapshots.
SELECT 1;
