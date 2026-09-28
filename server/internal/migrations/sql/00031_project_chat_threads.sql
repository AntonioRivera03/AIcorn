-- +goose Up
-- A project has any number of chats now, listed by most recent message.
DROP INDEX project_chat_active;
ALTER TABLE project_chat ADD COLUMN title TEXT NOT NULL DEFAULT '';
ALTER TABLE project_chat ADD COLUMN updatedAt TEXT NOT NULL DEFAULT '';
UPDATE project_chat SET updatedAt=COALESCE((SELECT MAX(createdAt) FROM project_chat_turn WHERE conversation=project_chat.id), createdAt);
UPDATE project_chat SET title=COALESCE((SELECT substr(trim(message),1,60) FROM project_chat_turn WHERE conversation=project_chat.id ORDER BY id LIMIT 1), '');
-- The single-chat era created a chat on first view; empty ones have nothing to list.
DELETE FROM project_chat WHERE NOT EXISTS(SELECT 1 FROM project_chat_turn WHERE conversation=project_chat.id);
CREATE INDEX project_chat_recent ON project_chat(project, updatedAt) WHERE archivedAt IS NULL;
-- A turn's work for the timeline: thinking and tool calls, as a JSON array.
ALTER TABLE project_chat_turn ADD COLUMN activityJson TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE project_chat_turn DROP COLUMN activityJson;
DROP INDEX project_chat_recent;
-- Only one chat per project can stay open again: keep each project's newest.
UPDATE project_chat SET archivedAt=CURRENT_TIMESTAMP WHERE archivedAt IS NULL AND id NOT IN (SELECT MAX(id) FROM project_chat WHERE archivedAt IS NULL GROUP BY project);
ALTER TABLE project_chat DROP COLUMN updatedAt;
ALTER TABLE project_chat DROP COLUMN title;
CREATE UNIQUE INDEX project_chat_active ON project_chat(project) WHERE archivedAt IS NULL;
