package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

// Exercise the real subprocess, framing, correlation and cancellation paths.
func TestAppServerPeer(t *testing.T) {
	if os.Getenv("AYCORN_TEST_PEER") != "1" {
		return
	}
	mode := os.Getenv("TEST_PEER_MODE")
	emit := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	event := func(method string, p any) { emit(map[string]any{"method": method, "params": p}) }
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var m struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			os.Exit(2)
		}
		result := any(map[string]any{})
		switch m.Method {
		case "initialized":
			continue
		case "thread/start", "thread/resume":
			if m.Method == "thread/start" && (mode == "resume" || m.Params["ephemeral"] != false) {
				os.Exit(3)
			}
			if m.Method == "thread/resume" && (m.Params["threadId"] != "thread-root" || m.Params["excludeTurns"] != true || m.Params["ephemeral"] != nil) {
				os.Exit(3)
			}
			if m.Params["modelProvider"] != "openai" {
				os.Exit(3)
			}
			result = map[string]any{"thread": map[string]string{"id": "thread-root"}}
			raw, _ := json.Marshal(m.Params)
			_ = os.WriteFile(os.Getenv("TEST_PEER_RECORD"), raw, 0600)
		case "turn/start":
			result = map[string]any{"turn": map[string]string{"id": "turn-root"}}
			emit(map[string]any{"id": m.ID, "result": result})
			if mode == "malformed" {
				fmt.Println("not json")
				os.Exit(0)
			}
			event("turn/started", map[string]any{"threadId": "thread-root", "turn": map[string]string{"id": "turn-root"}})
			event("item/completed", map[string]any{"threadId": "thread-root", "item": map[string]string{"type": "agentMessage", "text": "partial", "phase": "commentary"}})
			if mode == "partial" {
				os.Exit(0)
			}
			if mode == "cancel" {
				child := exec.Command("sleep", "30")
				_ = child.Start()
				_ = child.Wait()
				continue
			}
			if mode == "interactive" {
				emit(map[string]any{"id": 999, "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "thread-root"}})
				continue
			}
			event("item/completed", map[string]any{"threadId": "child", "item": map[string]string{"type": "agentMessage", "text": "child output"}})
			event("turn/completed", map[string]any{"threadId": "child", "turn": map[string]string{"status": "completed"}})
			event("item/completed", map[string]any{"threadId": "thread-root", "item": map[string]string{"type": "agentMessage", "text": "final answer", "phase": "final_answer"}})
			event("thread/tokenUsage/updated", map[string]any{"threadId": "thread-root", "turnId": "turn-root", "tokenUsage": map[string]any{"total": map[string]int{"inputTokens": 5}}})
			status := "completed"
			if mode == "failed" {
				status = "failed"
			}
			event("turn/completed", map[string]any{"threadId": "thread-root", "turn": map[string]string{"id": "turn-root", "status": status}})
			continue
		}
		emit(map[string]any{"id": m.ID, "result": result})
	}
	os.Exit(0)
}

func fakeAppServer(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test process wrapper")
	}
	t.Setenv("TEST_PEER_MODE", mode)
	t.Setenv("TEST_PEER_RECORD", filepath.Join(t.TempDir(), "request.json"))
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return makeFakeScript(t, "export AYCORN_TEST_PEER=1\nexec '"+strings.ReplaceAll(binary, "'", "'\\''")+"' -test.run=TestAppServerPeer -- \"$@\"")
}
func testRequest(executable string) *models.AIRunRequest {
	return &models.AIRunRequest{Engine: "codex", Executable: executable, Model: "gpt-5.6-sol", Intent: "ask", TimeoutSeconds: 10}
}

func TestCodexAppServerLifecycle(t *testing.T) {
	for _, mode := range []string{"complete", "partial", "failed", "malformed", "interactive"} {
		t.Run(mode, func(t *testing.T) {
			path := fakeAppServer(t, mode)
			var session, turn string
			result, err := (&Codex{FleetDir: t.TempDir()}).Run(context.Background(), RunSpec{TaskID: 42, Request: testRequest(path), WorkDir: t.TempDir(), OnSession: func(s, v string) error { session, turn = s, v; return nil }})
			if (err == nil) != (mode == "complete") {
				t.Fatalf("%+v %v", result, err)
			}
			if session != "thread-root" || turn != "turn-root" || result.SessionID != session {
				t.Fatalf("session checkpoint lost: %+v %s/%s", result, session, turn)
			}
			if mode == "complete" && (result.Output != "final answer" || !strings.Contains(result.UsageJson, "inputTokens")) {
				t.Fatalf("bad result: %+v", result)
			}
			if mode == "partial" && result.Output != "partial" {
				t.Fatal("lost partial response", result)
			}
		})
	}
}
func TestCodexCancellationKillsDescendants(t *testing.T) {
	path := fakeAppServer(t, "cancel")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := (&Codex{FleetDir: t.TempDir()}).Run(ctx, RunSpec{WorkDir: t.TempDir(), Request: testRequest(path)})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation hung: %v", err)
	}
	if result.Output != "partial" {
		t.Fatal("lost output", result)
	}
}
func TestCodexContextAndScope(t *testing.T) {
	for _, intent := range []string{"ask", "plan", "implement", "review"} {
		t.Run(intent, func(t *testing.T) {
			path := fakeAppServer(t, "complete")
			req := testRequest(path)
			req.Intent = intent
			req.RepoPath = "/repo"
			req.ProjectID = 7
			req.SystemPrompt = "custom instructions"
			req.Conductor = &models.ConductorRun{Phase: "planning", Settings: models.ConductorSettings{PlanningStage: 11, WorkingStage: 12, CompletionStage: 13}}
			_, err := (&Codex{MCPExecutable: "/bin/mcp", DBPath: "/data/app.db", FleetDir: t.TempDir()}).Run(context.Background(), RunSpec{Request: req, TaskID: 42, WorkDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(os.Getenv("TEST_PEER_RECORD"))
			if err != nil {
				t.Fatal(err)
			}
			var params map[string]any
			if err = json.Unmarshal(raw, &params); err != nil {
				t.Fatal(err)
			}
			sandbox := "read-only"
			if intent == "implement" {
				sandbox = "workspace-write"
			}
			if params["sandbox"] != sandbox {
				t.Fatal(params)
			}
			config := params["config"].(map[string]any)
			if config["agents.enabled"] != true {
				t.Fatal("subagents disabled")
			}
			if config["agents.max_concurrent_threads_per_session"] != float64(4) || config["mcp_servers.aycorn.required"] != true || config["mcp_servers.aycorn.command"] != "/bin/mcp" {
				t.Fatal("missing native orchestration or MCP configuration", config)
			}
			for _, role := range fleet.All() {
				path, ok := config["agents."+role.Role+".config_file"].(string)
				if !ok || !filepath.IsAbs(path) {
					t.Fatalf("missing executable profile for %s", role.Role)
				}
				body, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(body), "developer_instructions = ") {
					t.Fatalf("profile %s unavailable: %v", role.Role, err)
				}
			}
			for _, name := range []string{"read_task", "search_tasks", "project_context", "read_project_document"} {
				exposed := false
				for _, tool := range config["mcp_servers.aycorn.enabled_tools"].([]any) {
					exposed = exposed || tool == name
				}
				if !exposed || config["mcp_servers.aycorn.tools."+name+".approval_mode"] != "approve" {
					t.Fatal("MCP tool unavailable", name)
				}
			}
			skills := config["skills.config"].([]any)
			skill := skills[0].(map[string]any)
			if _, err := os.Stat(skill["path"].(string)); err != nil || skill["enabled"] != true {
				t.Fatal("root workflow skill unavailable", skill, err)
			}
			env := config["mcp_servers.aycorn.env"].(map[string]any)
			if env["AYCORN_RUN_TASK"] != "42" || env["AYCORN_CONDUCTOR_PROJECT"] != "7" {
				t.Fatal(env)
			}
			developer := params["developerInstructions"].(string)
			conductor, _ := fleet.Lookup("conductor")
			if !strings.Contains(developer, conductor.Instructions()) {
				t.Fatal("root did not load the complete Conductor profile")
			}
			if strings.Contains(developer, "custom instructions") {
				t.Fatal("custom prompt replaced fixed Conductor")
			}
			for _, want := range []string{"default project orchestrator", "planning=11, doing=12, review=13", "Only you manage the ticket"} {
				if !strings.Contains(developer, want) {
					t.Fatal(developer)
				}
			}
		})
	}
}
func TestCodexRejectsIncompatibleRequests(t *testing.T) {
	for _, req := range []*models.AIRunRequest{nil, {Model: "gpt-5.6-sol"}, {Engine: "codex", Model: "anthropic/claude"}} {
		if _, err := (&Codex{FleetDir: t.TempDir()}).Run(context.Background(), RunSpec{Request: req}); err == nil {
			t.Fatal("accepted incompatible request")
		}
	}
	if _, err := (&Registry{}).Run(context.Background(), RunSpec{Request: &models.AIRunRequest{Engine: "opencode"}}); err == nil {
		t.Fatal("OpenCode must be disabled")
	}
}

func TestCodexResumesChatWithCurrentContextAndScope(t *testing.T) {
	path := fakeAppServer(t, "resume")
	req := testRequest(path)
	req.Chat = &models.ChatTurn{SessionID: "thread-root"}
	result, err := (&Codex{FleetDir: t.TempDir()}).Run(context.Background(), RunSpec{Request: req, TaskID: 42, WorkDir: t.TempDir()})
	if err != nil || result.SessionID != "thread-root" || result.Output != "final answer" {
		t.Fatalf("resume: %+v %v", result, err)
	}
	raw, err := os.ReadFile(os.Getenv("TEST_PEER_RECORD"))
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err = json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	if params["sandbox"] != "read-only" || !strings.Contains(params["developerInstructions"].(string), "human-led ticket chat") {
		t.Fatalf("lost chat contract: %s", raw)
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

func TestMCPApprovalsAreLimitedToExposedScopedTools(t *testing.T) {
	h := &Codex{}
	spec := RunSpec{TaskID: 42, JobID: 7, Request: &models.AIRunRequest{ProjectID: 1}}
	config := h.sessionConfig(spec)
	if config["mcp_servers.aycorn.tools.add_task_link.approval_mode"] != "approve" {
		t.Fatal("authorized local writes would be rejected")
	}
	if _, ok := config["mcp_servers.aycorn.tools.update_task.approval_mode"]; ok {
		t.Fatal("task agent gained project mutations")
	}
	spec.Request.ProjectChat = &models.ProjectChatTurn{TurnID: 8, ConversationID: 2}
	spec.TaskID = 0
	spec.JobID = 0
	config = h.sessionConfig(spec)
	env := config["mcp_servers.aycorn.env"].(map[string]string)
	if env["AYCORN_CHAT_TURN"] != "8" || env["AYCORN_CHAT_PROJECT"] != "1" || env["AYCORN_RUN_TASK"] != "" {
		t.Fatal(env)
	}
	if config["mcp_servers.aycorn.tools.create_task.approval_mode"] != "approve" {
		t.Fatal(config)
	}
	developer, _ := BuildContext(spec)
	if strings.Contains(developer, "human-led ticket chat") || !strings.Contains(developer, "Chatter") {
		t.Fatal(developer)
	}
}
