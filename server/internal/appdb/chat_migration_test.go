package appdb_test

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

func TestChatMigrationEnablesExistingAndNewProjectsWithoutReplacingDefault(t *testing.T) {
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
	if err = goose.UpTo(db, "sql", 19); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO workflow(id,name) VALUES(1,'Test')`, `INSERT INTO project(id,name,workflow) VALUES(1,'Existing',1)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r := &repos.TaskTypeRepo{DB: db}
	defaultID, err := r.DefaultTypeID()
	if err != nil {
		t.Fatal(err)
	}
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO project(id,name,workflow) VALUES(2,'New',1)`); err != nil {
		t.Fatal(err)
	}
	if err = r.AddDefaultTypeToProject(2); err != nil {
		t.Fatal(err)
	}
	var chatID int
	if err = db.QueryRow("SELECT id FROM task_type WHERE viewMode='chat'").Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	for _, project := range []int{1, 2} {
		var enabled bool
		if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM project_task_type WHERE project=? AND task_type=?)", project, chatID).Scan(&enabled); err != nil || !enabled {
			t.Fatalf("project %d chat not enabled: %v", project, err)
		}
	}
	if current, err := r.DefaultTypeID(); err != nil || current != defaultID {
		t.Fatal("migration changed the default new-task type")
	}
	if _, err = db.Exec("UPDATE task_type SET name='Discussion' WHERE id=?", chatID); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err = db.QueryRow("SELECT viewMode FROM task_type WHERE id=?", chatID).Scan(&mode); err != nil || mode != "chat" {
		t.Fatal("renaming disabled chat")
	}
}
