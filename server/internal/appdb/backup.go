package appdb

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
)

// defaultBackupKeep is how many rotating snapshots to retain in the backups dir
// when AYCORN_BACKUP_KEEP is unset.
const defaultBackupKeep = 10

// ResolveBackupDir returns the directory snapshots live in: a "backups" folder
// alongside the DB file. So `make dev` snapshots land in server/backups/ and an
// installed binary's land next to its app.db under the OS config dir.
func ResolveBackupDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

// TimestampedName builds a snapshot filename like app-20060102-150405<suffix>.db.
func TimestampedName(suffix string) string {
	return fmt.Sprintf("app-%s%s.db", time.Now().Format("20060102-150405"), suffix)
}

// BackupKeep reads the retention count from AYCORN_BACKUP_KEEP (0 = keep all),
// falling back to defaultBackupKeep.
func BackupKeep() int {
	if v := os.Getenv("AYCORN_BACKUP_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultBackupKeep
}

// Snapshot writes a consistent, defragmented copy of the live DB to dest using
// SQLite's VACUUM INTO (safe even while the DB is in use, journal-mode agnostic).
// VACUUM INTO refuses to overwrite, so we guard explicitly for a clearer error.
func Snapshot(db *sql.DB, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("refusing to overwrite existing file %s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := db.Exec("VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("snapshot to %s: %w", dest, err)
	}
	return nil
}

// RotateBackups keeps only the newest keep snapshots matching app-*.db in dir,
// deleting the rest. keep <= 0 means keep everything. The timestamp in each
// filename is fixed-width, so a lexical sort is chronological.
func RotateBackups(dir string, keep int) {
	if keep <= 0 {
		return
	}
	matches, err := filepath.Glob(filepath.Join(dir, "app-*.db"))
	if err != nil {
		log.Printf("backup rotation: %v", err)
		return
	}
	if len(matches) <= keep {
		return
	}
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-keep] {
		if err := os.Remove(old); err != nil {
			log.Printf("backup rotation: could not remove %s: %v", old, err)
		}
	}
}

// LatestMigrationVersion returns the highest migration version embedded in the
// binary, parsed from the leading NNNNN of each sql/*.sql filename.
func LatestMigrationVersion() (int64, error) {
	entries, err := fs.ReadDir(migrations.Files, "sql")
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		idx := strings.IndexByte(name, '_')
		if idx <= 0 {
			continue
		}
		n, err := strconv.ParseInt(name[:idx], 10, 64)
		if err != nil {
			continue
		}
		if n > latest {
			latest = n
		}
	}
	return latest, nil
}

// BackupBeforeMigrate snapshots the DB before any pending migration runs, so an
// upgrade can never silently lose data. No-op when the DB is already at the
// latest version. On failure it returns an error — the caller aborts startup
// rather than migrate unprotected.
func BackupBeforeMigrate(db *sql.DB, dbPath string) error {
	current, err := goose.GetDBVersion(db)
	if err != nil {
		return err
	}
	latest, err := LatestMigrationVersion()
	if err != nil {
		return err
	}
	if current >= latest {
		return nil
	}
	dir := ResolveBackupDir(dbPath)
	dest := filepath.Join(dir, TimestampedName(fmt.Sprintf("-pre-v%d", latest)))
	if err := Snapshot(db, dest); err != nil {
		return fmt.Errorf("pre-migration backup failed (refusing to migrate unprotected): %w", err)
	}
	RotateBackups(dir, BackupKeep())
	log.Printf("Pre-migration backup written to %s (db v%d → v%d)", dest, current, latest)
	return nil
}
