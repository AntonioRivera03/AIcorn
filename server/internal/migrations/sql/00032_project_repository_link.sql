-- +goose Up
-- A project reaches its code one of two ways: 'personal' (a local checkout
-- the user keeps up to date, repoPath) or 'official' (a GitHub repository
-- Aycorn clones and fetches itself, repoUrl). '' means no repository yet.
-- The modes are validated in Go (internal/repolink), not by a CHECK
-- constraint, so a future mode needs no table rebuild.
ALTER TABLE project ADD COLUMN repoMode TEXT NOT NULL DEFAULT '';
ALTER TABLE project ADD COLUMN repoUrl TEXT NOT NULL DEFAULT '';

-- Existing local paths become Personal links. setProjectTimeModified bumps
-- timeModified whenever an update leaves it unchanged, so the backfill writes
-- a marked copy and then restores it: the projects keep their real
-- last-modified time.
UPDATE project SET repoMode = 'personal', timeModified = timeModified || ' '
WHERE TRIM(repoPath) <> '';
UPDATE project SET timeModified = substr(timeModified, 1, length(timeModified) - 1)
WHERE repoMode = 'personal' AND timeModified LIKE '% ';

-- The last clone or fetch of an Official link. Kept apart from project so a
-- fetch never bumps the project's timeModified. url records which link the
-- row describes; a row for an older link is ignored.
CREATE TABLE repository_sync (
    project   INTEGER PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
    url       TEXT NOT NULL,
    fetchedAt INTEGER,
    error     TEXT NOT NULL DEFAULT ''
);

-- A preview built from a remote-tracking branch (origin/<name>) of an
-- Official clone, rather than a local branch.
ALTER TABLE task_environment ADD COLUMN remote INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE task_environment DROP COLUMN remote;
DROP TABLE repository_sync;
ALTER TABLE project DROP COLUMN repoUrl;
ALTER TABLE project DROP COLUMN repoMode;
