-- +goose Up
-- Seed Coder persona with write-capable tool set (opencode harness, code-implementation agent).
-- Uses OR IGNORE to stay idempotent if placeholder.sql already inserted id=2.
INSERT OR IGNORE INTO persona (id, name, system_prompt, harness, model, agent, allowed_tools) VALUES
(2, 'Coder', '[{"type":"p","children":[{"text":"You are a coding agent working inside Aycorn. Read ticket context via MCP tools and make minimal, focused changes. Never auto-merge without human review."}]}]', 'opencode', 'opencode-go/muse-spark-1.2-contributor', 'code-implementation', '["search_tasks","read_task","list_projects","list_workflow_stages","list_checklists","list_task_types","create_task","update_task","move_task_stage"]');

-- Normalize legacy Research persona if it still uses claude-code/sonnet from older seed.
UPDATE persona SET harness = 'opencode', model = 'opencode-go/muse-spark-1.2-contributor', agent = 'research' WHERE id = 1 AND harness = 'claude-code';

-- +goose Down
DELETE FROM persona WHERE id = 2 AND name = 'Coder';
