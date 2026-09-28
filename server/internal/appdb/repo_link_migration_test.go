package appdb_test

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
)

// Projects with a local repo folder become Personal links; the rest have no
// repository. The backfill must not touch the projects' last-modified time.
func TestRepoLinkMigrationBackfillsPersonalLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "sql", 31); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Test')`,
		`INSERT INTO project(id,name,workflow,repoPath,timeModified) VALUES
			(1,'Linked',1,'/home/me/app','2024-03-01 10:00:00'),
			(2,'Blank',1,'','2024-03-02 11:00:00'),
			(3,'Spaces',1,'   ','2024-03-03 12:00:00')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]struct{ mode, modified string }{
		1: {"personal", "2024-03-01 10:00:00"},
		2: {"", "2024-03-02 11:00:00"},
		3: {"", "2024-03-03 12:00:00"},
	} {
		var mode, url, repoPath, modified string
		if err = db.QueryRow(`SELECT repoMode, repoUrl, repoPath, datetime(timeModified) FROM project WHERE id=?`, id).Scan(&mode, &url, &repoPath, &modified); err != nil {
			t.Fatal(err)
		}
		if mode != want.mode || url != "" || modified != want.modified {
			t.Fatalf("project %d: mode %q url %q modified %q; want %q, %q", id, mode, url, modified, want.mode, want.modified)
		}
		if id == 1 && repoPath != "/home/me/app" {
			t.Fatalf("repoPath lost: %q", repoPath)
		}
	}
	var remote int
	if err = db.QueryRow(`SELECT COUNT(*) FROM repository_sync`).Scan(&remote); err != nil || remote != 0 {
		t.Fatalf("repository_sync: %d, %v", remote, err)
	}
	// Later edits still bump timeModified through the existing trigger.
	if _, err = db.Exec(`UPDATE project SET name='Renamed' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var modified string
	if err = db.QueryRow(`SELECT datetime(timeModified) FROM project WHERE id=1`).Scan(&modified); err != nil || modified == "2024-03-01 10:00:00" {
		t.Fatalf("trigger no longer bumps timeModified: %q, %v", modified, err)
	}
}
