// Package accounts is the control plane of multi-user Aycorn: accounts,
// sessions, workspaces (personal and organization), memberships, and invites.
//
// It owns its own SQLite database (accounts.db in the data directory),
// separate from the per-workspace databases that hold projects and tasks.
// Nothing in here knows about projects; the workspaces package maps a
// workspace ID to its data.
package accounts

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DBFileName is the accounts database's file name inside the data directory.
const DBFileName = "accounts.db"

// Open opens (creating if needed) and migrates the accounts database in
// dataDir. It uses a goose Provider rather than goose's package-level state,
// because the workspace databases run their own migration set through the
// global goose API at the same time.
func Open(ctx context.Context, dataDir string) (*sql.DB, error) {
	dbPath := filepath.Join(dataDir, DBFileName)
	db, err := appdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	migrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		db.Close()
		return nil, err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := provider.Up(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
