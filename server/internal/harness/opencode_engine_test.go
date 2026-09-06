package harness

import (
	"context"
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeEventsRequireSuccessfulTerminal(t *testing.T) {
	cases := []struct {
		name, events string
		ok           bool
	}{
		{"complete", `{"type":"text","part":{"text":"rate limit is a normal topic"}}
{"type":"step_finish","part":{"reason":"stop","cost":0.1,"tokens":{"input":5}}}`, true},
		{"partial", `{"type":"text","part":{"text":"partial"}}`, false},
		{"provider error", `{"type":"text","part":{"text":"partial"}}
{"type":"error","error":{"message":"unauthorized"}}`, false},
		{"malformed", `not json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script := makeFakeScript(t, "cat <<'EVENTS'\n"+c.events+"\nEVENTS")
			req := &models.AIRunRequest{Executable: script, Model: "test/model", Intent: "ask", TimeoutSeconds: 10}
			h := &OpenCode{MCPExecutable: "/test/aycorn-mcp", DBPath: "/test/app.db"}
			result, err := h.Run(context.Background(), RunSpec{TaskID: 1, Request: req, WorkDir: t.TempDir()})
			if (err == nil) != c.ok {
				t.Fatalf("result %+v error %v", result, err)
			}
			if c.name == "partial" && !strings.Contains(result.Output, "partial") {
				t.Fatal("lost partial output")
			}
		})
	}
}
func TestOpenCodePolicyAndModel(t *testing.T) {
	for _, intent := range []string{"ask", "plan", "implement", "review"} {
		req := &models.AIRunRequest{Model: "chosen/model", Intent: intent, RepoPath: "/repo"}
		cfg, err := engineConfig(RunSpec{TaskID: 42, Request: req}, &OpenCode{MCPExecutable: "/bin/aycorn-mcp", DBPath: "/data/app.db"})
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
			t.Fatal(err)
		}
		permissions := parsed["permission"].(map[string]any)
		if parsed["model"] != "chosen/model" || permissions["*"] != "deny" || permissions["external_directory"] != "deny" {
			t.Fatal(cfg)
		}
		if (permissions["edit"] == "allow") != (intent == "implement") {
			t.Fatal(cfg)
		}
		if !strings.Contains(cfg, `"AYCORN_RUN_TASK":"42"`) {
			t.Fatal(cfg)
		}
	}
}
func TestOpenCodeCancelKillsDescendants(t *testing.T) {
	script := makeFakeScript(t, "echo '{\"type\":\"text\",\"part\":{\"text\":\"partial\"}}'\nsleep 30 &\nwait")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	h := &OpenCode{}
	result, err := h.Run(ctx, RunSpec{WorkDir: t.TempDir(), Request: &models.AIRunRequest{Executable: script, Model: "test/model", TimeoutSeconds: 10}})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation hung: %v", err)
	}
	if !strings.Contains(result.Output, "partial") {
		t.Fatal("lost cancellation output")
	}
}
