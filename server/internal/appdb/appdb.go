// Package appdb resolves the SQLite DB path, opens the DB with the app's
// standard pragmas, and applies startup migrations. Shared by cmd/web and
// cmd/mcp so both binaries agree on where the DB lives and how it's opened.
package appdb

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
)

// accountsDBFileName mirrors internal/accounts.DBFileName. appdb can't import
// internal/accounts (accounts imports appdb, so that would be a cycle), so
// the file name — and the one read-only query in listWorkspaces below — are
// duplicated here. Keep them in sync with internal/accounts/migrations if the
// `workspace` table's shape ever changes.
const accountsDBFileName = "accounts.db"

// WorkspaceDBPath returns the database path for the workspace with the given
// id inside dataDir. This is the single definition of the on-disk layout,
// shared by cmd/web's workspace registry (workspaceRegistry.dbPath) and
// ResolveDBPath's $AYCORN_WORKSPACE lookup below.
func WorkspaceDBPath(dataDir string, id int64) string {
	return filepath.Join(dataDir, "workspaces", strconv.FormatInt(id, 10), "app.db")
}

// ResolveDBPath returns the SQLite file path for a single-database consumer
// (cmd/mcp, and the `aycorn backup`/`aycorn restore` subcommands). cmd/web
// itself never calls this — it serves every workspace at once via
// ResolveDataDir and WorkspaceDBPath.
//
// Precedence:
//  1. $AYCORN_DB — explicit file override (ad-hoc use, and how
//     internal/harness points an agent run's cmd/mcp at its own workspace).
//  2. $AYCORN_WORKSPACE=<id> — resolves to that workspace's database under
//     ResolveDataDir() via WorkspaceDBPath. The workspace's data directory
//     must already exist (the server creates it when the workspace is
//     provisioned); this never creates one.
//  3. Neither variable set, but ResolveDataDir() is a multi-user data
//     directory (accounts.db present) — ambiguous, so this errors and lists
//     the available workspaces rather than guessing one.
//  4. Otherwise, a pre-accounts single-user install: <data dir>/app.db.
//     macOS:   ~/Library/Application Support/aycorn/app.db
//     Linux:   ~/.config/aycorn/app.db   (or $XDG_CONFIG_HOME/aycorn/app.db)
//     Windows: %AppData%\aycorn\app.db
func ResolveDBPath() (string, error) {
	if p := os.Getenv("AYCORN_DB"); p != "" {
		return p, nil
	}

	dataDir, err := ResolveDataDir()
	if err != nil {
		return "", err
	}

	if raw := os.Getenv("AYCORN_WORKSPACE"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return "", fmt.Errorf("invalid AYCORN_WORKSPACE %q: must be a positive workspace id", raw)
		}
		dbPath := WorkspaceDBPath(dataDir, id)
		if _, err := os.Stat(filepath.Dir(dbPath)); err != nil {
			return "", fmt.Errorf("no data directory for workspace %d: %s does not exist", id, filepath.Dir(dbPath))
		}
		return dbPath, nil
	}

	if _, err := os.Stat(filepath.Join(dataDir, accountsDBFileName)); err == nil {
		return "", ambiguousWorkspaceError(dataDir)
	}

	return filepath.Join(dataDir, "app.db"), nil
}

// ambiguousWorkspaceError builds the error ResolveDBPath returns when
// dataDir holds multiple users/workspaces and neither AYCORN_DB nor
// AYCORN_WORKSPACE says which one to use. It best-effort lists the
// workspaces to save the caller a trip to the UI; a listing failure still
// returns the actionable part of the message.
func ambiguousWorkspaceError(dataDir string) error {
	msg := "this data directory holds workspaces, not one database; set AYCORN_WORKSPACE=<id> to choose one"
	if lines, err := listWorkspaces(filepath.Join(dataDir, accountsDBFileName)); err == nil && len(lines) > 0 {
		msg += ":\n" + strings.Join(lines, "\n")
	}
	return errors.New(msg)
}

// listWorkspaces reads id/name/kind from accounts.db for
// ambiguousWorkspaceError, formatted as "id  name (kind)" per line. It opens
// the DB read-only, so it can't corrupt accounts.db even if this package's
// copy of the schema has drifted.
func listWorkspaces(accountsPath string) ([]string, error) {
	db, err := sql.Open("sqlite", "file:"+accountsPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, name, kind FROM workspace ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var id int64
		var name, kind string
		if err := rows.Scan(&id, &name, &kind); err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("  %d  %s (%s)", id, name, kind))
	}
	return lines, rows.Err()
}

// ResolveDataDir returns the directory multi-user Aycorn keeps its data in:
// accounts.db plus one sub-directory per workspace (see cmd/web).
//
// Precedence:
//  1. $AYCORN_DATA_DIR — explicit override (`make dev-test` points it at a
//     disposable server/data directory).
//  2. <os.UserConfigDir()>/aycorn — the default for installed binaries. A
//     single-user app.db from before accounts existed may sit here too; it is
//     left untouched.
func ResolveDataDir() (string, error) {
	dir := os.Getenv("AYCORN_DATA_DIR")
	if dir == "" {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cfgDir, "aycorn")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir, nil
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
