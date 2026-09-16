-- +goose Up
-- Original files live in the database so project deletion and backups are atomic.
CREATE TABLE project_document_file (
    documentId INTEGER PRIMARY KEY REFERENCES project_document(id) ON DELETE CASCADE,
    fileName TEXT NOT NULL,
    mediaType TEXT NOT NULL,
    byteSize INTEGER NOT NULL CHECK(byteSize > 0 AND byteSize <= 20971520),
    content BLOB NOT NULL
);

-- +goose Down
DROP TABLE project_document_file;
