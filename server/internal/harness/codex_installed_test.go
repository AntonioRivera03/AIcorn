package harness

import (
	"context"
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in protocol smoke test. Creates and re-reads a persistent session in an
// isolated CODEX_HOME without starting a model turn or using the user's chats.
func TestInstalledCodexProtocol(t *testing.T) {
	executable := os.Getenv("AYCORN_CODEX_INTEGRATION")
	if executable == "" {
		t.Skip("set AYCORN_CODEX_INTEGRATION to the installed executable")
	}
	mcp := os.Getenv("AYCORN_MCP_INTEGRATION")
	if mcp == "" {
		t.Fatal("set AYCORN_MCP_INTEGRATION to the built MCP executable")
	}
	root := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	if err := os.MkdirAll(os.Getenv("CODEX_HOME"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := &Codex{MCPExecutable: mcp, DBPath: filepath.Join(root, "app.db"), FleetDir: filepath.Join(root, "fleet")}
	spec := RunSpec{Request: testRequest(executable), TaskID: 1, WorkDir: root}
	dir, err := h.installFleet(spec)
	if err != nil {
		t.Fatal(err)
	}
	config := h.agentConfig(spec, dir)
	open := func() *rpcClient {
		client, err := startRPC(ctx, executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(root), root, func(rpcMessage) {})
		if err != nil {
			t.Fatal(err)
		}
		if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn_test", "version": "1.0"}}, nil); err != nil {
			client.close()
			t.Fatal(err)
		}
		if err = client.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
			client.close()
			t.Fatal(err)
		}
		return client
	}
	client := open()
	var started struct {
		Thread struct {
			ID        string `json:"id"`
			Ephemeral bool   `json:"ephemeral"`
		} `json:"thread"`
	}
	err = client.call(ctx, "thread/start", map[string]any{"cwd": root, "model": "gpt-5.6-sol", "modelProvider": "openai", "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": false, "config": config}, &started)
	if err != nil {
		client.close()
		t.Fatal(err)
	}
	if started.Thread.ID == "" || started.Thread.Ephemeral {
		client.close()
		t.Fatal("session not persisted", started)
	}
	err = client.call(ctx, "thread/name/set", map[string]string{"threadId": started.Thread.ID, "name": "Aycorn protocol verification"}, nil)
	client.close()
	if err != nil {
		t.Fatal(err)
	}
	client = open()
	defer client.close()
	var read struct {
		Thread struct {
			ID, Name string
			Source   any `json:"source"`
		} `json:"thread"`
	}
	if err = client.call(ctx, "thread/read", map[string]any{"threadId": started.Thread.ID, "includeTurns": false}, &read); err != nil {
		t.Fatal(err)
	}
	if read.Thread.ID != started.Thread.ID || read.Thread.Name != "Aycorn protocol verification" {
		t.Fatalf("session did not survive server restart: %+v", read)
	}
}

// Real model turns verify MCP, the bundled fleet/config, event mapping, resume
// and default native session listing. It is opt-in because it uses local login.
func TestInstalledCodexTurn(t *testing.T) {
	if os.Getenv("AYCORN_CODEX_LIVE") != "1" {
		t.Skip("set AYCORN_CODEX_LIVE=1 for a real model turn")
	}
	executable, mcp := os.Getenv("AYCORN_CODEX_INTEGRATION"), os.Getenv("AYCORN_MCP_INTEGRATION")
	if executable == "" || mcp == "" {
		t.Fatal("set the integration executable paths")
	}
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.db")
	db, err := appdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, dbPath); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO workflow(id,name) VALUES(1,'Test')`, `INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1)`, `INSERT INTO project(id,workflow,name) VALUES(1,1,'Protocol test')`, `INSERT INTO checklist(id,project,name) VALUES(1,1,'Test')`, `INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Protocol verification','Medium','')`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	req := testRequest(executable)
	req.TimeoutSeconds = 120
	req.TaskName = "Protocol verification"
	req.ProjectID = 1
	req.Instruction = "Delegate exactly one bounded task to the custom planner agent: read task 1 through Aycorn MCP and report its title. Wait for that agent, then reply exactly AYCORN_HARNESS_OK. Do not edit files."
	h := &Codex{MCPExecutable: mcp, DBPath: dbPath, FleetDir: filepath.Join(root, "fleet")}
	result, err := h.Run(context.Background(), RunSpec{Request: req, TaskID: 1, WorkDir: root})
	if err != nil {
		t.Fatalf("live run: %v; output: %s", err, result.Output)
	}
	if strings.TrimSpace(result.Output) != "AYCORN_HARNESS_OK" || result.SessionID == "" || result.TurnID == "" {
		t.Fatalf("unexpected live result: %+v", result)
	}
	req.Chat = &models.ChatTurn{SessionID: result.SessionID}
	req.Instruction = "Reply with exactly the final answer you gave to my previous message in this conversation. Do not delegate or edit anything."
	continued, err := h.Run(context.Background(), RunSpec{Request: req, TaskID: 1, WorkDir: root})
	if err != nil || continued.SessionID != result.SessionID || continued.TurnID == result.TurnID || strings.TrimSpace(continued.Output) != "AYCORN_HARNESS_OK" {
		t.Fatalf("conversation did not resume: %+v %v", continued, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := startRPC(ctx, executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(root), root, func(rpcMessage) {})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn_test", "version": "1.0"}}, nil); err != nil {
		t.Fatal(err)
	}
	_ = client.send(map[string]any{"method": "initialized", "params": map[string]any{}})
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = client.call(ctx, "thread/list", map[string]any{"cwd": root, "limit": 25}, &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, thread := range listed.Data {
		if thread.ID == result.SessionID {
			found = true
		}
	}
	if !found {
		t.Fatal("completed conversation missing from native listing")
	}
	var history json.RawMessage
	if err = client.call(ctx, "thread/read", map[string]any{"threadId": result.SessionID, "includeTurns": true}, &history); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "collabAgentToolCall") && !strings.Contains(string(history), "subAgentActivity") {
		t.Fatal("run did not exercise subagent delegation")
	}
	// Archive only the disposable conversation created by this test.
	if err = client.call(ctx, "thread/archive", map[string]string{"threadId": result.SessionID}, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("Verified persistent conversation %s, turn %s, then archived test conversation", result.SessionID, result.TurnID)
}
