-- +goose Up
-- View behavior is stable metadata; renaming the Chat type cannot disable chats.
ALTER TABLE task_type ADD COLUMN viewMode TEXT NOT NULL DEFAULT 'document' CHECK(viewMode IN ('document','chat'));
INSERT INTO task_type(name,description,icon,color,isDefault,category,viewMode)
SELECT 'Chat','Work through a ticket in a persistent AI conversation','messages-square','blue',0,id,'chat'
FROM task_type_category WHERE isDefault=1 LIMIT 1;
INSERT INTO project_task_type(project,task_type)
SELECT p.id,tt.id FROM project p CROSS JOIN task_type tt WHERE tt.viewMode='chat';
CREATE UNIQUE INDEX agent_job_chat_request ON agent_job(task,json_extract(requestJson,'$.chat.clientKey'))
WHERE json_type(requestJson,'$.chat')='object';

-- +goose Down
DROP INDEX agent_job_chat_request;
-- Preserve tasks and transcript history if the view feature is rolled back.
ALTER TABLE task_type DROP COLUMN viewMode;
