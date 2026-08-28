package models_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/mcptools"
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
		CREATE TABLE persona (
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
			Harness:      models.PersonaHarnessOpencode,
			Model:        models.PersonaModelMuseSpark12Contributor,
			Agent:        models.PersonaAgentCodeImplementation,
			AllowedTools: coderAllowedTools,
		})
		if err != nil {
			t.Fatalf("Create Coder: %v", err)
		}
		if created.Harness != models.PersonaHarnessOpencode {
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
			if !mcptools.IsAllowed(tool, created.AllowedTools) {
				t.Fatalf("Coder allowed_tools missing write tool %q: %v", tool, created.AllowedTools)
			}
		}
	})
}

func TestCoderVsResearchToolRestriction(t *testing.T) {
	t.Run("Research cannot call update_task even if model hallucinates", func(t *testing.T) {
		// FilterTools with research list must exclude update_task
		filtered := mcptools.FilterTools(researchAllowedTools)
		names := make([]string, len(filtered))
		for i, d := range filtered {
			names[i] = d.Name
		}
		if mcptools.IsAllowed("update_task", names) {
			t.Fatal("research FilterTools result must NOT contain update_task")
		}
		if mcptools.IsAllowed("create_task", names) {
			t.Fatal("research FilterTools result must NOT contain create_task")
		}
		if mcptools.IsAllowed("move_task_stage", names) {
			t.Fatal("research FilterTools result must NOT contain move_task_stage")
		}
		// IsAllowed directly on raw allowed list
		if mcptools.IsAllowed("update_task", researchAllowedTools) {
			t.Fatal("IsAllowed(update_task) should be false for research persona")
		}
	})

	t.Run("Coder can call update_task and create_task", func(t *testing.T) {
		filtered := mcptools.FilterTools(coderAllowedTools)
		names := make([]string, len(filtered))
		for i, d := range filtered {
			names[i] = d.Name
		}
		if !mcptools.IsAllowed("update_task", names) {
			t.Fatal("Coder FilterTools must contain update_task")
		}
		if !mcptools.IsAllowed("create_task", names) {
			t.Fatal("Coder FilterTools must contain create_task")
		}
		if !mcptools.IsAllowed("move_task_stage", names) {
			t.Fatal("Coder FilterTools must contain move_task_stage")
		}
		// Also IsAllowed on raw
		if !mcptools.IsAllowed("update_task", coderAllowedTools) {
			t.Fatal("IsAllowed(update_task) should be true for Coder")
		}
	})

	t.Run("GenerateMCPConfig for Research lacks update_task", func(t *testing.T) {
		dir := t.TempDir()
		path, cleanup, err := mcptools.GenerateMCPConfig(researchAllowedTools, dir)
		if err != nil {
			t.Fatalf("GenerateMCPConfig research: %v", err)
		}
		defer cleanup()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		raw := string(data)
		if strings.Contains(raw, "update_task") {
			t.Fatal("research GenerateMCPConfig must NOT contain update_task")
		}
		if strings.Contains(raw, "create_task") {
			t.Fatal("research GenerateMCPConfig must NOT contain create_task")
		}
		var cfg struct {
			Tools      []string `json:"tools"`
			MCPServers map[string]struct {
				AllowedTools []string `json:"allowedTools"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(cfg.Tools) != 3 {
			t.Fatalf("research tools len = %d; want 3", len(cfg.Tools))
		}
		if mcptools.IsAllowed("update_task", cfg.Tools) {
			t.Fatal("research cfg.Tools must not allow update_task")
		}
	})

	t.Run("GenerateMCPConfig for Coder includes update_task", func(t *testing.T) {
		dir := t.TempDir()
		path, cleanup, err := mcptools.GenerateMCPConfig(coderAllowedTools, dir)
		if err != nil {
			t.Fatalf("GenerateMCPConfig coder: %v", err)
		}
		defer cleanup()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		raw := string(data)
		if !strings.Contains(raw, "update_task") {
			t.Fatal("coder GenerateMCPConfig must contain update_task")
		}
		if !strings.Contains(raw, "create_task") {
			t.Fatal("coder GenerateMCPConfig must contain create_task")
		}
		if !strings.Contains(raw, "move_task_stage") {
			t.Fatal("coder GenerateMCPConfig must contain move_task_stage")
		}
		var cfg struct {
			Tools      []string `json:"tools"`
			MCPServers map[string]struct {
				AllowedTools []string `json:"allowedTools"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(cfg.Tools) != len(coderAllowedTools) {
			t.Fatalf("coder tools len = %d; want %d", len(cfg.Tools), len(coderAllowedTools))
		}
		srv := cfg.MCPServers["aycorn"]
		if !mcptools.IsAllowed("update_task", srv.AllowedTools) {
			t.Fatal("coder srv.AllowedTools must contain update_task")
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
		if !models.IsValidPersonaHarness(models.PersonaHarnessOpencode) {
			t.Fatal("opencode harness must be valid")
		}
		if !models.IsValidPersonaAgent(models.PersonaAgentCodeImplementation) {
			t.Fatal("code-implementation agent must be valid")
		}
		// Also general-senior is valid alternative per spec
		if !models.IsValidPersonaAgent(models.PersonaAgentGeneralSenior) {
			t.Fatal("general-senior agent must be valid")
		}
		if !models.IsValidPersonaModel(models.PersonaModelMuseSpark12Contributor) {
			t.Fatal("muse-spark-1.2-contributor model must be valid")
		}
	})
}
