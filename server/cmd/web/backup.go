package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
)

// runBackup handles `aycorn backup [dest]`: a clean, on-demand snapshot. With no
// dest it writes a rotating, timestamped file into the backups dir.
func runBackup(args []string) error {
	dbPath, err := appdb.ResolveDBPath()
	if err != nil {
		return err
	}
	if fi, err := os.Stat(dbPath); err != nil || fi.Size() == 0 {
		return fmt.Errorf("no database at %s to back up", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return err
	}
	defer db.Close()

	var dest string
	rotate := false
	if len(args) > 0 && args[0] != "" {
		dest = args[0]
	} else {
		dest = filepath.Join(appdb.ResolveBackupDir(dbPath), appdb.TimestampedName(""))
		rotate = true
	}

	if err := appdb.Snapshot(db, dest); err != nil {
		return err
	}
	if rotate {
		appdb.RotateBackups(appdb.ResolveBackupDir(dbPath), appdb.BackupKeep())
	}

	abs, err := filepath.Abs(dest)
	if err != nil {
		abs = dest
	}
	fmt.Printf("Backup written to %s\n", abs)
	return nil
}

// runRestore handles `aycorn restore <src>`: install a snapshot as the live DB
// (for moving to new hardware or rolling back). It validates the snapshot, backs
// up the current DB first so the restore itself is reversible, then swaps the
// file in. The next `aycorn` run migrates the schema forward via goose.
func runRestore(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("usage: aycorn restore <snapshot.db>")
	}
	src := args[0]
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("snapshot not found: %s", src)
	}

	// Best-effort guard; the pre-restore snapshot below is the real protection.
	if aycornRunning() {
		return fmt.Errorf("an aycorn server appears to be running — stop it first (`make stop` or `pkill aycorn`) before restoring")
	}

	if err := integrityCheck(src); err != nil {
		return err
	}

	dbPath, err := appdb.ResolveDBPath()
	if err != nil {
		return err
	}

	// Snapshot the current DB first so restore is reversible.
	if fi, err := os.Stat(dbPath); err == nil && fi.Size() > 0 {
		db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
		if err != nil {
			return err
		}
		dest := filepath.Join(appdb.ResolveBackupDir(dbPath), appdb.TimestampedName("-pre-restore"))
		err = appdb.Snapshot(db, dest)
		db.Close()
		if err != nil {
			return fmt.Errorf("could not back up current DB before restore: %w", err)
		}
		fmt.Printf("Backed up current database to %s\n", dest)
	}

	if err := copyFile(src, dbPath); err != nil {
		return err
	}
	fmt.Printf("Restored %s → %s\n", src, dbPath)
	fmt.Println("Stop any running server before restoring. Run `aycorn` to start; migrations will roll the schema forward.")
	return nil
}

// integrityCheck opens path as a SQLite DB and runs PRAGMA integrity_check.
func integrityCheck(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("could not read %s as a SQLite database: %w", path, err)
	}
	if result != "ok" {
		return fmt.Errorf("snapshot failed integrity check: %s", result)
	}
	return nil
}

// aycornRunning best-effort detects an installed aycorn binary by process name.
// It won't catch a `go run ./cmd/web` dev session (the process isn't named
// "aycorn"); the pre-restore snapshot covers that gap. The current process is
// excluded so `aycorn restore` doesn't flag itself.
func aycornRunning() bool {
	out, err := exec.Command("pgrep", "-x", "aycorn").Output()
	if err != nil {
		return false
	}
	self := os.Getpid()
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		if pid != self {
			return true
		}
	}
	return false
}

// copyFile copies src to dst, creating parent dirs as needed.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
