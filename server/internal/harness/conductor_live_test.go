package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"strings"
)

// Opt-in proof of dispatcher MCP -> independent root session -> same-session
// question. The answer exists only in a disposable project's document.
func TestInstalledConductorSessionsAndMCP(t *testing.T) {
	if os.Getenv("AYCORN_CONDUCTOR_LIVE") != "1" {
		t.Skip("set AYCORN_CONDUCTOR_LIVE=1 for real model turns")
	}
	executable, mcp := os.Getenv("AYCORN_CODEX_INTEGRATION"), os.Getenv("AYCORN_MCP_INTEGRATION")
	if executable == "" || mcp == "" {
		t.Fatal("set integration executable paths")
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
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Verification')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1),(2,1,'Work','doing','gray','circle',2),(3,1,'Review','todo','gray','circle',3)`,
		`INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Session verification',0)`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Verification',0)`,
		`INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Report the project token','Medium','[{"type":"p","children":[{"text":"Read the Project token document and report its exact token. This is a written research task. Do not edit anything or use subagents."}]}]')`,
		`INSERT INTO conductor_project(project,enabled,settings) VALUES(1,1,'{"enabled":true,"planningStage":1,"workingStage":2,"completionStage":3,"useRepository":false}')`,
		`INSERT INTO conductor_task(task,project,expectedStage) VALUES(1,1,1)`,
		`INSERT INTO conductor_dispatch(id,project,status,requestJson) VALUES(1,1,'running','{}')`,
		`UPDATE persona SET model='gpt-5.6-sol' WHERE builtin_role<>''`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("UPDATE ai_settings SET model='gpt-5.6-sol',executable=?,timeoutSeconds=120", executable); err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("AYCORN_SESSION_%d", time.Now().UnixNano())
	body, _ := json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": "The exact token is " + token}}}})
	if _, err = db.Exec("INSERT INTO project_document(id,project,title,body) VALUES(1,1,'Project token',?)", string(body)); err != nil {
		t.Fatal(err)
	}
	h := &Codex{MCPExecutable: mcp, DBPath: dbPath, FleetDir: filepath.Join(root, "fleet")}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	client, err := startRPC(ctx, executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(root), root, func(rpcMessage) {})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn_session_verification", "version": "1.0"}}, nil); err != nil {
		t.Fatal(err)
	}
	_ = client.send(map[string]any{"method": "initialized", "params": map[string]any{}})
	read := func(id string) conductorHistory {
		t.Helper()
		var v struct {
			Thread conductorHistory `json:"thread"`
		}
		if err := client.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true}, &v); err != nil {
			t.Fatal(err)
		}
		return v.Thread
	}
	archive := func(id string) { _ = client.call(ctx, "thread/archive", map[string]string{"threadId": id}, nil) }
	dispatch := testRequest(executable)
	dispatch.DispatchID = 1
	dispatch.ProjectID = 1
	dispatch.TimeoutSeconds = 120
	dispatch.Instruction = "Call list_conductor_tasks, then start_conductor_task with projectId=1, taskId=1, role=researcher. Do not defer this fully specified research task. Finish after the tool confirms it is queued."
	result, err := h.Run(ctx, RunSpec{Request: dispatch, WorkDir: root})
	if result.SessionID != "" {
		defer archive(result.SessionID)
	}
	if err != nil {
		t.Fatal("dispatcher", err, result.Output)
	}
	history := read(result.SessionID)
	calls := history.mcpCalls()
	if !calls["list_conductor_tasks"] || !calls["start_conductor_task"] {
		t.Fatal("dispatcher did not call scoped tools", calls)
	}
	var id, stage int
	var raw string
	if err = db.QueryRow("SELECT id,requestJson FROM agent_job WHERE task=1").Scan(&id, &raw); err != nil {
		t.Fatal(err)
	}
	var req models.AIRunRequest
	if err = json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if req.PresetName != "Research" || req.TaskSession == nil {
		t.Fatal("wrong independent role")
	}
	if err = db.QueryRow("SELECT stage FROM task WHERE id=1").Scan(&stage); err != nil || stage != 2 {
		t.Fatal("start tool did not move stage atomically", stage, err)
	}
	if _, err = db.Exec("UPDATE agent_job SET status='running' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	work, err := h.Run(ctx, RunSpec{Request: &req, TaskID: 1, JobID: id, WorkDir: root})
	if work.SessionID != "" {
		defer archive(work.SessionID)
	}
	if err != nil {
		t.Fatal("task work", err, work.Output)
	}
	if work.SessionID == result.SessionID || !strings.Contains(work.Output, token) {
		t.Fatal("session isolation or MCP evidence missing", work.Output)
	}
	taskHistory := read(work.SessionID)
	if !taskHistory.mcpCalls()["read_project_document"] {
		t.Fatal("task did not read document")
	}
	for _, h := range []conductorHistory{history, taskHistory} {
		for _, item := range h.items() {
			if item.Type == "collabAgentToolCall" || item.Type == "subAgentActivity" {
				t.Fatal("native child created", item)
			}
		}
	}
	req.Conductor = nil
	req.TaskSession.Mode = "question"
	req.Chat.SessionID = work.SessionID
	req.Instruction = "What exact token did you report? Answer from this conversation only; do not perform any new work."
	answer, err := h.Run(ctx, RunSpec{Request: &req, TaskID: 1, JobID: id, WorkDir: root})
	if err != nil || answer.SessionID != work.SessionID || !strings.Contains(answer.Output, token) {
		t.Fatal("follow-up did not retain session", answer, err)
	}
	t.Logf("Verified dispatcher %s, independent task %s, and resumed question", result.SessionID, work.SessionID)
}

type conductorHistory struct {
	AgentRole string `json:"agentRole"`
	Turns     []struct {
		Items []conductorHistoryItem `json:"items"`
	} `json:"turns"`
}

type conductorHistoryItem struct {
	Type, Tool, Status, Server string
	ReceiverThreadIDs          []string        `json:"receiverThreadIds"`
	AgentThreadID              string          `json:"agentThreadId"`
	Error                      json.RawMessage `json:"error"`
}

func (h conductorHistory) items() []conductorHistoryItem {
	var items []conductorHistoryItem
	for _, turn := range h.Turns {
		items = append(items, turn.Items...)
	}
	return items
}

func (h conductorHistory) mcpCalls() map[string]bool {
	calls := map[string]bool{}
	for _, item := range h.items() {
		if item.Type == "mcpToolCall" && item.Server == "aycorn" && item.Status == "completed" && (len(item.Error) == 0 || string(item.Error) == "null") {
			calls[item.Tool] = true
		}
	}
	return calls
}
