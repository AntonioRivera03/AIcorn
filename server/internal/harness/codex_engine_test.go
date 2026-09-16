package harness

import (
	"context"
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexEventsRequireSuccessfulTerminal(t *testing.T) {
	for _, c := range []struct {
		name, events string
		ok           bool
	}{
		{"complete", `{"type":"item.completed","item":{"type":"agent_message","text":"rate limit is a normal topic"}}
{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":3}}`, true},
		{"partial", `{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}`, false},
		{"provider error", `{"type":"turn.failed","error":{"message":"unauthorized"}}`, false},
		{"error after completion", `{"type":"item.completed","item":{"type":"agent_message","text":"answer"}}
{"type":"turn.completed"}
{"type":"error","message":"connection lost"}`, false},
		{"malformed", "not json", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			script := makeFakeScript(t, "cat <<'EVENTS'\n"+c.events+"\nEVENTS")
			req := &models.AIRunRequest{Engine: "codex", Executable: script, Model: "gpt-5.6-sol", Intent: "ask", TimeoutSeconds: 10}
			result, err := (&Codex{}).Run(context.Background(), RunSpec{TaskID: 1, Request: req, WorkDir: t.TempDir()})
			if (err == nil) != c.ok {
				t.Fatalf("%+v %v", result, err)
			}
			if c.name == "partial" && !strings.Contains(result.Output, "partial") {
				t.Fatal("lost partial result")
			}
		})
	}
}
func TestCodexScopesCommandsAndProvider(t *testing.T) {
	for _, intent := range []string{"ask", "plan", "implement", "review"} {
		req := &models.AIRunRequest{Engine: "codex", Model: "gpt-5.6-sol", Intent: intent, RepoPath: "/repo", SystemPrompt: "Custom instructions"}
		args, err := engineArgs(RunSpec{TaskID: 42, WorkDir: "/worktree", Request: req}, &Codex{MCPExecutable: "/bin/aycorn-mcp", DBPath: "/data/app.db"}, "/tmp/answer", "/tmp/schema")
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(args, "\n")
		sandbox := "read-only"
		if intent == "implement" {
			sandbox = "workspace-write"
		}
		for _, want := range []string{"--ignore-user-config", "--ignore-rules", "--sandbox\n" + sandbox, `model_provider="openai"`, `approval_policy="never"`, `sandbox_workspace_write.network_access=false`, `AYCORN_RUN_TASK="42"`, `mcp_servers.aycorn.tools.read_task.approval_mode="approve"`, `projects."/worktree".trust_level="untrusted"`, "Custom instructions"} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %s: %s", want, text)
			}
		}
		if strings.Contains(text, "danger-full-access") || strings.Contains(text, "openai_base_url") {
			t.Fatal(text)
		}
	}
}
func TestConductorPlannerScopesMCPAndSchema(t *testing.T) {
	req := &models.AIRunRequest{Engine: "codex", Intent: "plan", Model: "gpt-6-astra", ProjectID: 7, Conductor: &models.ConductorRun{Phase: "planning"}}
	args, err := engineArgs(RunSpec{TaskID: 42, Request: req}, &Codex{}, "answer", "schema")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, "\n")
	for _, want := range []string{`AYCORN_CONDUCTOR_PROJECT="7"`, `["read_task","search_tasks"]`, "--output-schema\nschema", "--sandbox\nread-only"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	var schema map[string]any
	if err = json.Unmarshal(conductorSchema("planning"), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatal(schema)
	}
}
func TestCodexRejectsLegacyAndOtherProviderRequests(t *testing.T) {
	for _, req := range []*models.AIRunRequest{nil, {Model: "gpt-5.6-sol"}, {Engine: "codex", Model: "anthropic/claude"}} {
		if _, err := engineArgs(RunSpec{Request: req}, &Codex{}, "", ""); err == nil {
			t.Fatal("accepted incompatible request")
		}
	}
}
func TestCodexCancellationKillsDescendants(t *testing.T) {
	script := makeFakeScript(t, `echo '{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}'
sleep 30 &
wait`)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := (&Codex{}).Run(ctx, RunSpec{WorkDir: t.TempDir(), Request: &models.AIRunRequest{Engine: "codex", Executable: script, Model: "gpt-5.6-sol", TimeoutSeconds: 10}})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation hung: %v", err)
	}
	if result.Output != "partial" {
		t.Fatal("lost output")
	}
}
func TestCodexFinalMessageOverridesCommentaryAndPinsDirectory(t *testing.T) {
	t.Setenv("PWD", "/wrong/parent")
	script := makeFakeScript(t, `while [ "$#" -gt 0 ]; do
 if [ "$1" = "--cd" ]; then shift; target="$1"; fi
 if [ "$1" = "--output-last-message" ]; then shift; result="$1"; fi
 shift
done
[ "$target" = "$PWD" ] || exit 12
[ "$target" = "$(pwd -P)" ] || exit 13
cat >/dev/null
echo '{"type":"item.completed","item":{"type":"agent_message","text":"intermediate prose"}}'
echo '{"type":"turn.completed","usage":{"input_tokens":5}}'
echo '{"ready":true,"context":"plan","missingContext":""}' > "$result"`)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&Codex{}).Run(context.Background(), RunSpec{WorkDir: dir, Request: &models.AIRunRequest{Engine: "codex", Executable: script, Intent: "ask", Model: "gpt-5.6-sol", TimeoutSeconds: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "intermediate") || !strings.Contains(result.Output, `"ready":true`) {
		t.Fatal(result)
	}
	if !strings.Contains(result.UsageJson, `"input_tokens":5`) {
		t.Fatal(result.UsageJson)
	}
}
func makeFakeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
