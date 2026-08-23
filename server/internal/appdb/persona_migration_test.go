package appdb_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	_ "modernc.org/sqlite"
)

func migratedTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := appdb.Migrate(db, dbPath); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func TestPersonaMigration_defaultsAllowedToolsToRestrictiveArray(t *testing.T) {
	db := migratedTestDB(t)

	result, err := db.Exec(`
		INSERT INTO persona (name, system_prompt, harness, model)
		VALUES ('Researcher', 'Research only', 'claude-code', 'sonnet')
	`)
	if err != nil {
		t.Fatalf("insert persona: %v", err)
	}
	personaID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read persona id: %v", err)
	}

	var allowedTools string
	if err := db.QueryRow("SELECT allowed_tools FROM persona WHERE id = ?", personaID).Scan(&allowedTools); err != nil {
		t.Fatalf("read allowed_tools: %v", err)
	}
	if allowedTools != "[]" {
		t.Fatalf("allowed_tools = %q; want []", allowedTools)
	}
}

func TestPersonaMigration_rejectsNonArrayAllowedTools(t *testing.T) {
	db := migratedTestDB(t)

	_, err := db.Exec(`
		INSERT INTO persona (name, system_prompt, harness, model, allowed_tools)
		VALUES ('Researcher', 'Research only', 'claude-code', 'sonnet', '{}')
	`)
	if err == nil {
		t.Fatal("expected non-array allowed_tools to violate the schema")
	}
}

func TestStagePersonaMigration_enforcesOnePersonaPerStage(t *testing.T) {
	db := migratedTestDB(t)
	stageID := insertStage(t, db)
	firstPersonaID := insertPersona(t, db, "First")
	secondPersonaID := insertPersona(t, db, "Second")

	if _, err := db.Exec("INSERT INTO stage_persona (stage_id, persona_id) VALUES (?, ?)", stageID, firstPersonaID); err != nil {
		t.Fatalf("bind first persona: %v", err)
	}
	if _, err := db.Exec("INSERT INTO stage_persona (stage_id, persona_id) VALUES (?, ?)", stageID, secondPersonaID); err == nil {
		t.Fatal("expected a second binding for one stage to violate the schema")
	}
}

func TestStagePersonaMigration_removesBindingWhenStageDeleted(t *testing.T) {
	db := migratedTestDB(t)
	stageID := insertStage(t, db)
	personaID := insertPersona(t, db, "Researcher")
	if _, err := db.Exec("INSERT INTO stage_persona (stage_id, persona_id) VALUES (?, ?)", stageID, personaID); err != nil {
		t.Fatalf("bind persona: %v", err)
	}

	if _, err := db.Exec("DELETE FROM stage WHERE id = ?", stageID); err != nil {
		t.Fatalf("delete stage: %v", err)
	}

	var bindingCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM stage_persona WHERE stage_id = ?", stageID).Scan(&bindingCount); err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("binding count after stage deletion = %d; want 0", bindingCount)
	}
}

func insertStage(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	workflow, err := db.Exec("INSERT INTO workflow (name) VALUES ('Test')")
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	workflowID, err := workflow.LastInsertId()
	if err != nil {
		t.Fatalf("read workflow id: %v", err)
	}
	stage, err := db.Exec(`
		INSERT INTO stage (workflow, name, color, icon, position, type)
		VALUES (?, 'Open', 'gray', 'circle-dashed', 1, 'open')
	`, workflowID)
	if err != nil {
		t.Fatalf("insert stage: %v", err)
	}
	stageID, err := stage.LastInsertId()
	if err != nil {
		t.Fatalf("read stage id: %v", err)
	}
	return stageID
}

func insertPersona(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	persona, err := db.Exec(`
		INSERT INTO persona (name, system_prompt, harness, model)
		VALUES (?, 'Research only', 'claude-code', 'sonnet')
	`, name)
	if err != nil {
		t.Fatalf("insert persona: %v", err)
	}
	personaID, err := persona.LastInsertId()
	if err != nil {
		t.Fatalf("read persona id: %v", err)
	}
	return personaID
}
