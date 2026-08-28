package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouting_ResearchUsesShim(t *testing.T) {
	shim := NewReadOnlyShim()
	realScript := makeFakeScript(t, `echo '{"is_error":false,"result":"real-output","total_cost_usd":0.01,"usage":{}}'`)
	real := NewOpencodeHarness(WithCLIPath(realScript))
	router := NewRoutingHarness(shim, real)

	spec := RunSpec{
		TaskID:       1,
		TaskName:     "Research task",
		SystemPrompt: "You are a research assistant",
		AllowedTools: []string{"search_tasks", "read_task"},
	}
	res, err := router.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "No filesystem was touched") {
		t.Fatalf("expected shim output containing 'No filesystem was touched', got %q", res.Output)
	}
	if strings.Contains(res.Output, "real-output") {
		t.Fatalf("should not have delegated to real: %q", res.Output)
	}
}

func TestRouting_CoderUsesReal(t *testing.T) {
	shim := NewReadOnlyShim()
	realScript := makeFakeScript(t, `echo '{"is_error":false,"result":"real-coder-output","total_cost_usd":0.01,"usage":{}}'`)
	real := NewOpencodeHarness(WithCLIPath(realScript))
	router := NewRoutingHarness(shim, real)

	spec := RunSpec{
		TaskID:       2,
		TaskName:     "Coder task",
		SystemPrompt: "You are a coding agent",
		AllowedTools: []string{"search_tasks", "read_task", "update_task"},
		WorkDir:      t.TempDir(),
	}
	res, err := router.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "real-coder-output") {
		t.Fatalf("expected real output 'real-coder-output', got %q", res.Output)
	}
	if strings.Contains(res.Output, "No filesystem was touched") {
		t.Fatalf("should not be shim: %q", res.Output)
	}
}

func TestRouting_CoderByToolCountUsesReal(t *testing.T) {
	shim := NewReadOnlyShim()
	realScript := makeFakeScript(t, `echo '{"is_error":false,"result":"real-many-tools","total_cost_usd":0.01,"usage":{}}'`)
	real := NewOpencodeHarness(WithCLIPath(realScript))
	router := NewRoutingHarness(shim, real)

	spec := RunSpec{
		TaskID:       3,
		AllowedTools: []string{"search_tasks", "read_task", "list_projects", "list_workflow_stages"},
		WorkDir:      t.TempDir(),
	}
	res, err := router.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "real-many-tools") {
		t.Fatalf("expected real-many-tools, got %q", res.Output)
	}
}

func TestRouting_CodingPromptUsesReal(t *testing.T) {
	shim := NewReadOnlyShim()
	realScript := makeFakeScript(t, `echo '{"is_error":false,"result":"real-coding-prompt","total_cost_usd":0.01,"usage":{}}'`)
	real := NewOpencodeHarness(WithCLIPath(realScript))
	router := NewRoutingHarness(shim, real)

	spec := RunSpec{
		TaskID:       4,
		SystemPrompt: "You are a coding assistant that writes files",
		WorkDir:      t.TempDir(),
	}
	res, err := router.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "real-coding-prompt") {
		t.Fatalf("expected real-coding-prompt, got %q", res.Output)
	}
}

func TestRouting_MCPConfigPassedToReal(t *testing.T) {
	shim := NewReadOnlyShim()
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{}}`), 0600); err != nil {
		t.Fatalf("write mcp: %v", err)
	}
	script := makeFakeScript(t, `echo "{\"is_error\":false,\"result\":\"$* $AYCORN_MCP_CONFIG\",\"total_cost_usd\":0.01,\"usage\":{}}"`)
	opencodePath := filepath.Join(dir, "fake-opencode-mcp")
	data, _ := os.ReadFile(script)
	if err := os.WriteFile(opencodePath, data, 0755); err != nil {
		t.Fatalf("copy: %v", err)
	}
	real := NewOpencodeHarness(WithCLIPath(opencodePath))
	router := NewRoutingHarness(shim, real)

	spec := RunSpec{
		TaskID:       5,
		WorkDir:      t.TempDir(),
		AllowedTools: []string{"update_task"},
		MCPConfigPath: mcpPath,
	}
	res, err := router.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "--mcp-config") {
		t.Fatalf("expected --mcp-config in args, got %q", res.Output)
	}
	if !strings.Contains(res.Output, mcpPath) {
		t.Fatalf("expected mcp path %q in output %q", mcpPath, res.Output)
	}
}
