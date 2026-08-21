// Package appdb resolves the SQLite DB path, opens the DB with the app's
// standard pragmas, and applies startup migrations. Shared by cmd/web and
// cmd/mcp so both binaries agree on where the DB lives and how it's opened.
package appdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
)

// ResolveDBPath returns the SQLite file path.
//
// Precedence:
//  1. $AYCORN_DB — explicit override (used by `make dev` and for ad-hoc testing
//     so dev builds don't clobber an installed user DB).
//  2. <os.UserConfigDir()>/aycorn/app.db — the default for installed binaries.
//     macOS:   ~/Library/Application Support/aycorn/app.db
//     Linux:   ~/.config/aycorn/app.db   (or $XDG_CONFIG_HOME/aycorn/app.db)
//     Windows: %AppData%\aycorn\app.db
func ResolveDBPath() (string, error) {
	if p := os.Getenv("AYCORN_DB"); p != "" {
		return p, nil
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, "aycorn")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "app.db"), nil
}

// Open opens the DB with the app's standard pragmas: foreign keys on, WAL
// journal mode (required so cmd/web and cmd/mcp can hold connections to the
// same file at once — see Documentation/ai-architecture.md §4), and a busy
// timeout so a writer that arrives mid-write retries instead of failing.
func Open(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
}

// Migrate snapshots the DB (if it exists and has pending migrations) and
// applies any pending goose migrations. Idempotent — safe to call from
// multiple binaries/processes on startup; goose.Up no-ops once at the latest
// version, so cmd/mcp does not need cmd/web to have run first. Runs under
// withFileLock so cmd/web and cmd/mcp launched at nearly the same moment
// can't both decide a migration is pending and race BackupBeforeMigrate +
// goose.Up against each other.
func Migrate(db *sql.DB, dbPath string) error {
	return withFileLock(dbPath, func() error {
		dbExisted := false
		if fi, err := os.Stat(dbPath); err == nil && fi.Size() > 0 {
			dbExisted = true
		}
		goose.SetBaseFS(migrations.Files)
		if err := goose.SetDialect("sqlite3"); err != nil {
			return err
		}
		if dbExisted {
			if err := BackupBeforeMigrate(db, dbPath); err != nil {
				return err
			}
		}
		return goose.Up(db, "sql")
	})
}

const (
	lockRetryInterval = 50 * time.Millisecond
	lockWaitTimeout   = 5 * time.Second
	// lockStaleAge is how old an unreleased lock file must be before a new
	// process treats it as abandoned (e.g. left behind by a process that
	// crashed mid-migration) rather than actively held.
	lockStaleAge = 30 * time.Second
)

// withFileLock serializes DB maintenance (migrations, backups) across the
// cmd/web and cmd/mcp processes that can both open dbPath at once. It's a
// plain lock file rather than flock(2)/LockFileEx, so it behaves identically
// on every OS this app ships for. Best-effort: a lock that can't be acquired
// within lockWaitTimeout doesn't block startup forever — fn still runs, with
// SQLite's own busy_timeout as the backstop against actual concurrent writes.
func withFileLock(dbPath string, fn func() error) error {
	lockPath := filepath.Join(filepath.Dir(dbPath), ".aycorn-maintenance.lock")
	deadline := time.Now().Add(lockWaitTimeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > lockStaleAge {
			os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fn()
		}
		time.Sleep(lockRetryInterval)
	}
}
