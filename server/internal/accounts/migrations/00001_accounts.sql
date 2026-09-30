-- +goose Up

-- The accounts database is the control plane: who can sign in, which
-- workspaces exist, and who belongs to them. Workspace data (projects, tasks,
-- workflows, …) lives in a separate SQLite file per workspace; see
-- server/internal/workspaces.

CREATE TABLE account (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL COLLATE NOCASE UNIQUE,
    name TEXT NOT NULL,
    passwordHash TEXT NOT NULL,
    -- The onboarding answer. NULL until the user has chosen, which is how the
    -- app knows to show onboarding after signup.
    usage TEXT CHECK(usage IN ('solo', 'organization')),
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose StatementBegin
CREATE TRIGGER setAccountTimeModified
AFTER UPDATE ON account
FOR EACH ROW
WHEN NEW.timeModified IS OLD.timeModified
BEGIN
    UPDATE account SET timeModified = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
-- +goose StatementEnd

-- Only a SHA-256 of the cookie token is stored, so a leaked database can't be
-- replayed as live sessions.
CREATE TABLE session (
    tokenHash TEXT PRIMARY KEY,
    account INTEGER NOT NULL,
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    timeExpires TIMESTAMP NOT NULL,

    FOREIGN KEY (account) REFERENCES account(id) ON DELETE CASCADE
);

CREATE INDEX sessionAccount ON session(account);

-- A workspace is either one account's personal space or an organization.
-- Every account gets exactly one personal workspace at signup.
CREATE TABLE workspace (
    -- AUTOINCREMENT so a deleted workspace's id (and so its data directory)
    -- is never handed to a new workspace.
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL CHECK(kind IN ('personal', 'organization')),
    name TEXT NOT NULL,
    -- Set only for personal workspaces.
    personalAccount INTEGER,
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (personalAccount) REFERENCES account(id) ON DELETE CASCADE,
    CHECK ((kind = 'personal') = (personalAccount IS NOT NULL))
);

CREATE UNIQUE INDEX onePersonalWorkspacePerAccount ON workspace(personalAccount) WHERE kind = 'personal';

-- +goose StatementBegin
CREATE TRIGGER setWorkspaceTimeModified
AFTER UPDATE ON workspace
FOR EACH ROW
WHEN NEW.timeModified IS OLD.timeModified
BEGIN
    UPDATE workspace SET timeModified = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
-- +goose StatementEnd

CREATE TABLE membership (
    workspace INTEGER NOT NULL,
    account INTEGER NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('owner', 'admin', 'member')),
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (workspace, account),
    FOREIGN KEY (workspace) REFERENCES workspace(id) ON DELETE CASCADE,
    FOREIGN KEY (account) REFERENCES account(id) ON DELETE CASCADE
);

CREATE INDEX membershipAccount ON membership(account);

-- An invite whitelists one email for one organization. The code is the only
-- secret (it's also what the invite link carries), so only its hash is kept.
CREATE TABLE invite (
    id INTEGER PRIMARY KEY,
    workspace INTEGER NOT NULL,
    email TEXT NOT NULL COLLATE NOCASE,
    role TEXT NOT NULL CHECK(role IN ('admin', 'member')),
    codeHash TEXT NOT NULL UNIQUE,
    invitedBy INTEGER,
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    timeExpires TIMESTAMP NOT NULL,
    timeAccepted TIMESTAMP,
    acceptedBy INTEGER,
    timeRevoked TIMESTAMP,

    FOREIGN KEY (workspace) REFERENCES workspace(id) ON DELETE CASCADE,
    FOREIGN KEY (invitedBy) REFERENCES account(id) ON DELETE SET NULL,
    FOREIGN KEY (acceptedBy) REFERENCES account(id) ON DELETE SET NULL
);

-- Re-inviting an email revokes its previous pending invite first.
CREATE UNIQUE INDEX onePendingInvitePerEmail ON invite(workspace, email)
    WHERE timeAccepted IS NULL AND timeRevoked IS NULL;

-- +goose Down

DROP TABLE invite;
DROP TABLE membership;
DROP TABLE workspace;
DROP TABLE session;
DROP TABLE account;
