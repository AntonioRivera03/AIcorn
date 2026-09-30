-- +goose Up

-- NULL until the account proves it owns its email address. Accounts created
-- before verification existed are trusted as they are.
ALTER TABLE account ADD COLUMN timeEmailVerified TIMESTAMP;
UPDATE account SET timeEmailVerified = COALESCE(timeCreated, CURRENT_TIMESTAMP);

-- One-time links emailed to an account: confirming its email address, or
-- resetting its password. Like session tokens, only a SHA-256 of the token is
-- stored.
CREATE TABLE account_token (
    tokenHash TEXT PRIMARY KEY,
    account INTEGER NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('verify_email', 'reset_password')),
    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    timeExpires TIMESTAMP NOT NULL,

    FOREIGN KEY (account) REFERENCES account(id) ON DELETE CASCADE
);

CREATE INDEX accountTokenAccount ON account_token(account, purpose);

-- +goose Down

DROP TABLE account_token;
ALTER TABLE account DROP COLUMN timeEmailVerified;
