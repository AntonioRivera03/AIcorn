package appdb_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	_ "modernc.org/sqlite"
)

func TestResolveDBPath_AycornDBWins(t *testing.T) {
	dataDir := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "explicit.db")
	t.Setenv("AYCORN_DATA_DIR", dataDir)
	t.Setenv("AYCORN_DB", explicit)
	t.Setenv("AYCORN_WORKSPACE", "1") // must be ignored once AYCORN_DB is set

	got, err := appdb.ResolveDBPath()
	if err != nil {
		t.Fatalf("ResolveDBPath: %v", err)
	}
	if got != explicit {
		t.Fatalf("ResolveDBPath() = %q; want %q", got, explicit)
	}
}

func TestResolveDBPath_WorkspaceResolvesUnderDataDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AYCORN_DATA_DIR", dataDir)
	t.Setenv("AYCORN_DB", "")
	t.Setenv("AYCORN_WORKSPACE", "3")

	if err := os.MkdirAll(filepath.Join(dataDir, "workspaces", "3"), 0o755); err != nil {
		t.Fatalf("seed workspace directory: %v", err)
	}

	got, err := appdb.ResolveDBPath()
	if err != nil {
		t.Fatalf("ResolveDBPath: %v", err)
	}
	// ResolveDataDir runs EvalSymlinks, so compare against its own resolved
	// output rather than the raw dataDir the test set AYCORN_DATA_DIR to.
	resolvedDataDir, err := appdb.ResolveDataDir()
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	want := appdb.WorkspaceDBPath(resolvedDataDir, 3)
	if got != want {
		t.Fatalf("ResolveDBPath() = %q; want %q", got, want)
	}
}

func TestResolveDBPath_MissingWorkspaceDirectoryErrors(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AYCORN_DATA_DIR", dataDir)
	t.Setenv("AYCORN_DB", "")
	t.Setenv("AYCORN_WORKSPACE", "9") // no workspaces/9 directory created

	if _, err := appdb.ResolveDBPath(); err == nil {
		t.Fatal("expected an error for a workspace with no data directory")
	}
}

func TestResolveDBPath_InvalidWorkspaceIDErrors(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("AYCORN_DATA_DIR", t.TempDir())
			t.Setenv("AYCORN_DB", "")
			t.Setenv("AYCORN_WORKSPACE", raw)

			if _, err := appdb.ResolveDBPath(); err == nil {
				t.Fatalf("expected AYCORN_WORKSPACE=%q to be rejected", raw)
			}
		})
	}
}

func TestResolveDBPath_MultiUserDataDirWithoutEitherVarErrors(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AYCORN_DATA_DIR", dataDir)
	t.Setenv("AYCORN_DB", "")
	t.Setenv("AYCORN_WORKSPACE", "")

	seedAccountsDB(t, dataDir)

	_, err := appdb.ResolveDBPath()
	if err == nil {
		t.Fatal("expected an error when a multi-user data directory has neither AYCORN_DB nor AYCORN_WORKSPACE set")
	}
	msg := err.Error()
	for _, want := range []string{"AYCORN_WORKSPACE", "1  Solo (personal)", "2  Acme (organization)"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not mention %q", msg, want)
		}
	}
}

// seedAccountsDB writes a minimal accounts.db to dataDir containing just the
// workspace table ResolveDBPath's ambiguity error reads from.
func seedAccountsDB(t *testing.T, dataDir string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "accounts.db"))
	if err != nil {
		t.Fatalf("open accounts fixture: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE workspace (id INTEGER PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL)`); err != nil {
		t.Fatalf("create workspace table: %v", err)
	}
	for _, row := range []struct {
		id   int
		name string
		kind string
	}{
		{1, "Solo", "personal"},
		{2, "Acme", "organization"},
	} {
		if _, err := db.Exec(`INSERT INTO workspace (id, name, kind) VALUES (?, ?, ?)`, row.id, row.name, row.kind); err != nil {
			t.Fatalf("seed workspace %d: %v", row.id, err)
		}
	}
}
