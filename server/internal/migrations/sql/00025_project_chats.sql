-- +goose Up
CREATE TABLE project_chat (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 project INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 sessionId TEXT NOT NULL DEFAULT '',
 createdAt TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 archivedAt TEXT
);
CREATE UNIQUE INDEX project_chat_active ON project_chat(project) WHERE archivedAt IS NULL;
CREATE TABLE project_chat_turn (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 conversation INTEGER NOT NULL REFERENCES project_chat(id) ON DELETE CASCADE,
 clientKey TEXT NOT NULL,
 message TEXT NOT NULL,
 requestJson TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','canceling','completed','failed','canceled','interrupted')),
 output TEXT NOT NULL DEFAULT '',
 progress TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '',
 sessionId TEXT NOT NULL DEFAULT '',
 turnId TEXT NOT NULL DEFAULT '',
 usageJson TEXT NOT NULL DEFAULT '{}',
 createdAt TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 finishedAt TEXT,
 UNIQUE(conversation,clientKey)
);
CREATE UNIQUE INDEX project_chat_one_turn ON project_chat_turn(conversation) WHERE status IN ('pending','running','canceling');

-- +goose Down
DROP TABLE project_chat_turn;
DROP TABLE project_chat;
