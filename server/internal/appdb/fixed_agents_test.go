package appdb_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

func TestFixedAgentMigrationPreservesPreferencesAndAdoptsExistingRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "sql", 25); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE persona SET model='gpt-6-astra', system_prompt='Preserve original coder prompt' WHERE id=2`,
		`INSERT INTO persona(id,name,harness,model,system_prompt) VALUES(50,'Research','codex','gpt-5.5','Preserve research prompt'),(51,'My custom agent','codex','gpt-5.6-luna','Preserve custom prompt')`,
		`INSERT INTO workflow(id,name) VALUES(1,'Workflow')`,
		`INSERT INTO project(id,name,workflow) VALUES(1,'Project',1),(2,'New project',1)`,
		`INSERT INTO conductor_project(project,enabled,settings) VALUES(1,0,'{"planningStage":12,"planningPrompt":"Project rules","conductorAgentId":51,"taskAgentId":51}')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	repo := &repos.PersonaRepo{DB: db}
	coder, err := repo.FindRole("coder")
	if err != nil || coder.ID != 2 || coder.Model != "gpt-6-astra" || !strings.Contains(coder.Instructions, "# Coder") {
		t.Fatal(coder, err)
	}
	research, err := repo.FindRole("researcher")
	if err != nil || research.ID != 50 || research.Model != "gpt-5.5" {
		t.Fatal(research, err)
	}
	for id, want := range map[int]string{2: "Preserve original coder prompt", 50: "Preserve research prompt", 51: "Preserve custom prompt"} {
		var prompt string
		if err = db.QueryRow("SELECT system_prompt FROM persona WHERE id=?", id).Scan(&prompt); err != nil || prompt != want {
			t.Fatal(id, prompt, err)
		}
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM persona WHERE builtin_role<>''").Scan(&count); err != nil || count != 6 {
		t.Fatal(count, err)
	}
	root, err := repo.FindRole("conductor")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []int{1, 2} {
		s, _, err := (&repos.ConductorRepo{DB: db}).Settings(project)
		if err != nil || s.ConductorAgentID != root.ID || s.TaskAgentID != coder.ID {
			t.Fatal(s, err)
		}
		if project == 1 && (s.PlanningStage != 12 || s.PlanningPrompt != "Project rules") {
			t.Fatal("changed project configuration", s)
		}
	}
}
