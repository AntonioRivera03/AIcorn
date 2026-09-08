-- +goose Up
-- Copy both sides before replacing the FK so preset deletion preserves history.
CREATE TABLE agent_job_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
 persona INTEGER REFERENCES persona(id) ON DELETE SET NULL,
 status TEXT NOT NULL, fromStage INTEGER, toStage INTEGER,
 claimedAt TEXT, startedAt TEXT, finishedAt TEXT,
 attempts INTEGER NOT NULL DEFAULT 0, error TEXT,
 createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
 requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT ''
);
INSERT INTO agent_job_new (id,task,persona,status,fromStage,toStage,claimedAt,startedAt,finishedAt,attempts,error,createdAt)
 SELECT id,task,persona,status,fromStage,toStage,claimedAt,startedAt,finishedAt,attempts,error,createdAt FROM agent_job;
CREATE TABLE agent_run_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 job INTEGER NOT NULL REFERENCES agent_job_new(id) ON DELETE CASCADE,
 output TEXT, summary TEXT, exitCode INTEGER, usageJson TEXT,
 createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
 artifactJson TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO agent_run_new (id,job,output,summary,exitCode,usageJson,createdAt)
 SELECT id,job,output,summary,exitCode,usageJson,createdAt FROM agent_run;
DROP TABLE agent_run;
DROP TABLE agent_job;
ALTER TABLE agent_job_new RENAME TO agent_job;
ALTER TABLE agent_run_new RENAME TO agent_run;
-- Never execute work queued under the superseded implicit contract.
UPDATE agent_job SET status='interrupted', error='Queued under the previous AI system. Start a new explicit run.',
 finishedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE status IN ('pending','claimed','running');
-- +goose StatementBegin
CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);
CREATE UNIQUE INDEX one_active_ai_job_per_task ON agent_job(task) WHERE status IN ('pending','claimed','running','canceling');
-- +goose StatementEnd
CREATE TABLE ai_settings (
 id INTEGER PRIMARY KEY CHECK (id=1),
 model TEXT NOT NULL DEFAULT '', executable TEXT NOT NULL DEFAULT '',
 timeoutSeconds INTEGER NOT NULL DEFAULT 300
);
INSERT INTO ai_settings (id, model) VALUES (1, COALESCE((SELECT model FROM persona ORDER BY id LIMIT 1), ''));

-- +goose Down
-- Forward-only: nullable preset ownership and preserved run history must not be undone.
SELECT 1;
