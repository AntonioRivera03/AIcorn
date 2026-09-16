-- +goose Up
-- Distinguish a fresh handoff from a released/re-added task during selection.
ALTER TABLE conductor_task ADD COLUMN selectionKey TEXT NOT NULL DEFAULT '';

-- Dispatcher runs are project decisions, never task sessions or task owners.
CREATE TABLE conductor_dispatch (
 id INTEGER PRIMARY KEY,
 project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 status TEXT NOT NULL CHECK(status IN ('running','completed','failed','interrupted')),
 requestJson TEXT NOT NULL,
 output TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '',
 sessionId TEXT NOT NULL DEFAULT '',
 createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX one_conductor_dispatch ON conductor_dispatch(project) WHERE status='running';
-- Retries of a task message may not enqueue a second turn.
CREATE UNIQUE INDEX task_session_request ON agent_job(task,json_extract(requestJson,'$.chat.clientKey'))
 WHERE json_type(requestJson,'$.taskSession')='object' AND json_extract(requestJson,'$.chat.clientKey') IS NOT NULL;

-- Upgrade only the old default; retain customized project instructions.
UPDATE conductor_project SET settings=json_set(settings,'$.planningPrompt','Inspect the tasks given to Conductor. Start eligible tasks with the appropriate independent agent. Defer tasks with unresolved dependencies or essential missing context, giving a specific reason.')
 WHERE json_extract(settings,'$.planningPrompt')='Validate the goal, acceptance criteria, dependencies, and required context. Add a concise plan and useful context without implementing the task. If anything essential is missing, ask specific questions instead of guessing.';

-- +goose Down
ALTER TABLE conductor_task DROP COLUMN selectionKey;
DROP INDEX task_session_request;
DROP TABLE conductor_dispatch;
