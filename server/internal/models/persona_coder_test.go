package models_test

import (
	"database/sql"
	"encoding/json"
	"slices"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

// coderAllowedTools is the canonical Coder persona tool set (must include writes).
var coderAllowedTools = []string{
	"search_tasks", "read_task", "list_projects", "list_workflow_stages",
	"list_checklists", "list_task_types", "create_task", "update_task", "move_task_stage",
}

var researchAllowedTools = []string{"read_task", "search_tasks", "list_projects"}

func coderTestApp(t *testing.T) (*sql.DB, *services.PersonaService) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE persona (builtin_role TEXT NOT NULL DEFAULT '',
		    id            INTEGER PRIMARY KEY AUTOINCREMENT,
		    name          TEXT NOT NULL DEFAULT '',
		    system_prompt TEXT NOT NULL DEFAULT '[]',
		    harness       TEXT NOT NULL DEFAULT 'opencode',
		    model         TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor',
		    agent         TEXT NOT NULL DEFAULT '',
		    allowed_tools TEXT NOT NULL DEFAULT '[]',
		    timeCreated   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		    timeModified  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		);
	`); err != nil {
		t.Fatalf("create persona table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &services.PersonaService{PersonaRepo: &repos.PersonaRepo{DB: db}}
	return db, svc
}

func TestCoderPersonaValidation_acceptsWriteTools(t *testing.T) {
	_, svc := coderTestApp(t)

	t.Run("create Coder with write tools passes validation", func(t *testing.T) {
		created, err := svc.Create(&models.Persona{
			Name:         "Coder",
			SystemPrompt: `[{"type":"p","children":[{"text":"You are a coding agent working inside Aycorn. Read ticket context via MCP tools and make minimal, focused changes. Never auto-merge without human review."}]}]`,
			Harness:      models.PersonaHarnessCodex,
			Model:        models.PersonaModelDefault,
			Agent:        models.PersonaAgentCodeImplementation,
			AllowedTools: coderAllowedTools,
		})
		if err != nil {
			t.Fatalf("Create Coder: %v", err)
		}
		if created.Harness != models.PersonaHarnessCodex {
			t.Fatalf("harness = %q; want opencode", created.Harness)
		}
		if created.Agent != models.PersonaAgentCodeImplementation {
			t.Fatalf("agent = %q; want code-implementation", created.Agent)
		}
		if !models.IsValidPersonaHarness(created.Harness) {
			t.Fatalf("IsValidPersonaHarness false for %q", created.Harness)
		}
		if !models.IsValidPersonaAgent(created.Agent) {
			t.Fatalf("IsValidPersonaAgent false for %q", created.Agent)
		}
		if !models.IsValidPersonaModel(created.Model) {
			t.Fatalf("IsValidPersonaModel false for %q", created.Model)
		}
		// Must include writes
		for _, tool := range []string{"create_task", "update_task", "move_task_stage"} {
			if !slices.Contains(created.AllowedTools, tool) {
				t.Fatalf("Coder allowed_tools missing write tool %q: %v", tool, created.AllowedTools)
			}
		}
	})
}

func TestCoderAllowedToolsJSONRoundTrip(t *testing.T) {
	t.Run("EncodeAllowedTools and ParseAllowedTools round-trip for Coder", func(t *testing.T) {
		encoded, err := models.EncodeAllowedTools(coderAllowedTools)
		if err != nil {
			t.Fatalf("EncodeAllowedTools: %v", err)
		}
		// Encoded must be valid JSON array
		var arr []string
		if err := json.Unmarshal([]byte(encoded), &arr); err != nil {
			t.Fatalf("encoded not JSON array: %v", err)
		}
		decoded, err := models.ParseAllowedTools(encoded)
		if err != nil {
			t.Fatalf("ParseAllowedTools: %v", err)
		}
		if len(decoded) != len(coderAllowedTools) {
			t.Fatalf("round-trip len = %d; want %d", len(decoded), len(coderAllowedTools))
		}
		for i, want := range coderAllowedTools {
			if decoded[i] != want {
				t.Fatalf("round-trip[%d] = %q; want %q", i, decoded[i], want)
			}
		}
		// Empty round-trip must be []
		empty, _ := models.EncodeAllowedTools([]string{})
		if empty != "[]" {
			t.Fatalf("empty encode = %q; want []", empty)
		}
		parsed, err := models.ParseAllowedTools(empty)
		if err != nil {
			t.Fatalf("Parse empty: %v", err)
		}
		if len(parsed) != 0 {
			t.Fatalf("parsed empty len = %d; want 0", len(parsed))
		}
	})

	t.Run("EncodeAllCatalogTools round-trip preserves order", func(t *testing.T) {
		// Use catalog order for deterministic check
		all := []string{"search_tasks", "read_task", "list_projects", "list_workflow_stages", "list_checklists", "list_task_types", "create_task", "update_task", "move_task_stage"}
		encoded, err := models.EncodeAllowedTools(all)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		decoded, err := models.ParseAllowedTools(encoded)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		for i := range all {
			if decoded[i] != all[i] {
				t.Fatalf("order mismatch at %d: got %q want %q", i, decoded[i], all[i])
			}
		}
	})
}

func TestCoderModelAndAgentConstants(t *testing.T) {
	t.Run("Coder uses opencode harness and valid agent", func(t *testing.T) {
		if !models.IsValidPersonaHarness(models.PersonaHarnessCodex) {
			t.Fatal("opencode harness must be valid")
		}
		if !models.IsValidPersonaAgent(models.PersonaAgentCodeImplementation) {
			t.Fatal("code-implementation agent must be valid")
		}
		// Also general-senior is valid alternative per spec
		if !models.IsValidPersonaAgent(models.PersonaAgentGeneralSenior) {
			t.Fatal("general-senior agent must be valid")
		}
		if !models.IsValidPersonaModel(models.PersonaModelDefault) {
			t.Fatal("muse-spark-1.2-contributor model must be valid")
		}
	})
}
