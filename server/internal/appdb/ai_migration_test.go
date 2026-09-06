package appdb_test

import (
	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
	"path/filepath"
	"testing"
)

func TestExplicitRunMigrationPreservesLegacyHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "sql", 14); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Test')`,
		`INSERT INTO stage(id,workflow,name,color,icon,position,type) VALUES(1,1,'Open','gray','circle',1,'open')`,
		`INSERT INTO project(id,name,workflow) VALUES(1,'Test',1)`,
		`INSERT INTO checklist(id,name,project) VALUES(1,'Test',1)`,
		`INSERT INTO task(id,name,checklist,stage,type,priority) VALUES(1,'Test',1,1,1,'Medium')`,
		`INSERT INTO persona(id,name,harness,model) VALUES(100,'Legacy','opencode','test/model')`,
		`INSERT INTO agent_job(id,task,persona,status) VALUES(7,1,100,'completed'),(8,1,100,'pending')`,
		`INSERT INTO agent_run(id,job,output,usageJson) VALUES(9,7,'Existing answer','legacy malformed usage')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM persona WHERE id=100`); err != nil {
		t.Fatal(err)
	}
	var output, usage, status string
	if err := db.QueryRow(`SELECT output,usageJson FROM agent_run WHERE id=9 AND job=7`).Scan(&output, &usage); err != nil {
		t.Fatal(err)
	}
	if output != "Existing answer" || usage != "legacy malformed usage" {
		t.Fatal("history changed")
	}
	if err := db.QueryRow(`SELECT status FROM agent_job WHERE id=8 AND persona IS NULL`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "interrupted" {
		t.Fatal("legacy queued work remained runnable")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left invalid foreign keys")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
