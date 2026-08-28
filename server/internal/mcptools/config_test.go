package mcptools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGenerateFiltersResearchOnly(t *testing.T) {
	// Research persona: read-only tools only.
	researchTools := []string{"read_task", "search_tasks", "list_projects"}

	dir := t.TempDir()
	path, cleanup, err := GenerateMCPConfig(researchTools, dir)
	if err != nil {
		t.Fatalf("GenerateMCPConfig: %v", err)
	}
	defer cleanup()

	// Verify file exists with 0600 permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw := string(data)

	// Must contain allowed tools.
	if !strings.Contains(raw, "search_tasks") {
		t.Error("config should contain search_tasks")
	}
	if !strings.Contains(raw, "read_task") {
		t.Error("config should contain read_task")
	}
	if !strings.Contains(raw, "list_projects") {
		t.Error("config should contain list_projects")
	}

	// Must NOT contain write tools even if model hallucinates them.
	if strings.Contains(raw, "update_task") {
		t.Error("research config must NOT contain update_task — hallucinated write must be absent")
	}
	if strings.Contains(raw, "create_task") {
		t.Error("research config must NOT contain create_task")
	}
	if strings.Contains(raw, "move_task_stage") {
		t.Error("research config must NOT contain move_task_stage")
	}

	// Structured check: parse and verify filtered counts.
	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Tools) != 3 {
		t.Errorf("tools len = %d, want 3", len(cfg.Tools))
	}
	srv, ok := cfg.MCPServers["aycorn"]
	if !ok {
		t.Fatal("missing mcpServers.aycorn")
	}
	if len(srv.AllowedTools) != 3 {
		t.Errorf("allowedTools len = %d, want 3", len(srv.AllowedTools))
	}
	for _, want := range researchTools {
		if !IsAllowed(want, srv.AllowedTools) {
			t.Errorf("IsAllowed(%q) = false, want true", want)
		}
	}
	if IsAllowed("update_task", srv.AllowedTools) {
		t.Error("IsAllowed(update_task) should be false for research persona")
	}

	// FilterTools helper directly.
	filtered := FilterTools(researchTools)
	if len(filtered) != 3 {
		t.Errorf("FilterTools len = %d, want 3", len(filtered))
	}
	names := make([]string, len(filtered))
	for i, d := range filtered {
		names[i] = d.Name
	}
	if IsAllowed("update_task", names) {
		t.Error("FilterTools result should not contain update_task")
	}

	// Cleanup removes file.
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("cleanup should remove temp file")
	}
}

func TestEmptyMeansNoTools(t *testing.T) {
	dir := t.TempDir()

	// Empty slice.
	path, cleanup, err := GenerateMCPConfig([]string{}, dir)
	if err != nil {
		t.Fatalf("GenerateMCPConfig(empty): %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Tools) != 0 {
		t.Errorf("empty allowed => tools len %d, want 0 (must not grant all)", len(cfg.Tools))
	}
	srv := cfg.MCPServers["aycorn"]
	if len(srv.AllowedTools) != 0 {
		t.Errorf("empty allowed => allowedTools len %d, want 0", len(srv.AllowedTools))
	}
	if len(FilterTools([]string{})) != 0 {
		t.Error("FilterTools(empty) should return 0")
	}
	if len(FilterTools(nil)) != 0 {
		t.Error("FilterTools(nil) should return 0")
	}
	// Raw JSON must encode as [] not null.
	if strings.Contains(string(data), "null") {
		t.Error("config should encode empty tools as [], not null")
	}

	// Ensure no tool names leak into empty config.
	for _, name := range orderedNames {
		if strings.Contains(string(data), string(name)) {
			t.Errorf("empty config should not contain %q", string(name))
		}
	}
}

func TestUnknownToolsSkipped(t *testing.T) {
	dir := t.TempDir()
	path, cleanup, err := GenerateMCPConfig([]string{"search_tasks", "does_not_exist", "read_task"}, dir)
	if err != nil {
		t.Fatalf("GenerateMCPConfig: %v", err)
	}
	defer cleanup()

	data, _ := os.ReadFile(path)
	var cfg mcpConfigFile
	_ = json.Unmarshal(data, &cfg)
	if len(cfg.Tools) != 2 {
		t.Errorf("unknown tool should be skipped: got %d tools, want 2", len(cfg.Tools))
	}
	if strings.Contains(string(data), "does_not_exist") {
		t.Error("unknown tool name should not appear in config")
	}
}

func TestIsAllowed(t *testing.T) {
	allowed := []string{"search_tasks", "read_task"}
	if !IsAllowed("search_tasks", allowed) {
		t.Error("expected allowed")
	}
	if IsAllowed("update_task", allowed) {
		t.Error("expected not allowed")
	}
	if IsAllowed("search_tasks", nil) {
		t.Error("nil allowed should not allow anything")
	}
	if IsAllowed("search_tasks", []string{}) {
		t.Error("empty allowed should not allow anything")
	}
}
