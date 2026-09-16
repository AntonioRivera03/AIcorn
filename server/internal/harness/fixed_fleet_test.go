package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

func TestFixedFleetUsesPerRoleModelsAndRejectsPromptOverrides(t *testing.T) {
	req := testRequest("codex")
	req.AgentModels = map[string]string{"conductor": "gpt-6-astra", "planner": "gpt-5.5", "researcher": "gpt-5.6-luna", "coder": "gpt-5.6-sol", "reviewer": "gpt-5.6-terra", "chatter": "gpt-5.5"}
	req.Conductor = &models.ConductorRun{TaskAgent: &models.AgentSnapshot{Model: "gpt-5.6-sol", Instructions: "REPLACE_THE_FIXED_PROMPT"}}
	h := &Codex{FleetDir: t.TempDir()}
	path, err := h.installFleet(RunSpec{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range fleet.All() {
		b, err := os.ReadFile(filepath.Join(path, d.Role+".toml"))
		if err != nil {
			t.Fatal(err)
		}
		config := string(b)
		if !strings.Contains(config, "model = "+tomlString(req.AgentModels[d.Role])) || strings.Contains(config, "REPLACE_THE_FIXED_PROMPT") || !strings.Contains(config, "developer_instructions = "+tomlString(d.Instructions())) {
			t.Fatalf("wrong %s model or instructions: %s", d.Role, config)
		}
		if !strings.Contains(config, "[[skills.config]]") || !strings.Contains(config, fleet.WorkflowPath) {
			t.Fatal("missing workflow skill", d.Role)
		}
		if d.ReadOnly && !strings.Contains(config, `sandbox_mode = "read-only"`) {
			t.Fatal("missing sandbox", d.Role)
		}
	}
	req.AgentModels["reviewer"] = "gpt-6-astra"
	updated, err := h.installFleet(RunSpec{Request: req})
	if err != nil || updated == path {
		t.Fatal("model change reused stale fleet", updated, err)
	}
	old, err := os.ReadFile(filepath.Join(path, "reviewer.toml"))
	if err != nil || !strings.Contains(string(old), `model = "gpt-5.6-terra"`) {
		t.Fatal("changed active fleet", err)
	}
}
