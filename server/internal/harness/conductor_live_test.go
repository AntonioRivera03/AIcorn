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
	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

// Opt-in end-to-end proof of actual Conductor delegation and inherited MCP.
// The fixture's answer exists only in a project document in a disposable DB.
// Assertions inspect native child histories, not claims in the final response.
func TestInstalledConductorDelegationAndMCP(t *testing.T) {
	if os.Getenv("AYCORN_CONDUCTOR_LIVE") != "1" {
		t.Skip("set AYCORN_CONDUCTOR_LIVE=1 to use local Codex login for real model turns")
	}
	executable, mcp := os.Getenv("AYCORN_CODEX_INTEGRATION"), os.Getenv("AYCORN_MCP_INTEGRATION")
	if executable == "" || mcp == "" {
		t.Fatal("set the integration executable paths")
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
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
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Plan','open','gray','circle',1),(2,1,'Work','doing','gray','circle',2),(3,1,'Review','todo','gray','circle',3)`,
		`INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Conductor verification',0)`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Verification',0)`,
		`INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Write the project token','Medium','[{"type":"p","children":[{"text":"Create proof.txt containing exactly the token in the Project token document, followed by a newline. Use the registered subagents. Reviewer must verify the file against the document. Do not create any other files or change tickets."}]}]')`,
		`INSERT INTO agent_job(id,task,status) VALUES(1,1,'running')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	token := fmt.Sprintf("AYCORN_MCP_%d", time.Now().UnixNano())
	body, _ := json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": "The exact token is " + token}}}})
	if _, err = db.Exec("INSERT INTO project_document(id,project,title,body) VALUES(1,1,'Project token',?)", string(body)); err != nil {
		t.Fatal(err)
	}
	h := &Codex{MCPExecutable: mcp, DBPath: dbPath, FleetDir: filepath.Join(root, "fleet")}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	client, err := startRPC(ctx, executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(work), work, func(rpcMessage) {})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn_conductor_verification", "version": "1.0"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err = client.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	read := func(id string) conductorHistory {
		t.Helper()
		var response struct {
			Thread conductorHistory `json:"thread"`
		}
		if err := client.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true}, &response); err != nil {
			t.Fatal(err)
		}
		return response.Thread
	}
	for _, phase := range []string{"planning", "working"} {
		req := testRequest(executable)
		req.TimeoutSeconds, req.ProjectID = 240, 1
		req.TaskName = "Write the project token"
		req.AgentModels = map[string]string{}
		for _, role := range fleet.All() {
			req.AgentModels[role.Role] = req.Model
		}
		req.Conductor = &models.ConductorRun{Phase: phase, Settings: models.ConductorSettings{PlanningStage: 1, WorkingStage: 2, CompletionStage: 3}}
		roles := []string{"planner", "researcher"}
		req.Intent = "plan"
		req.Instruction = "Assess readiness of the assigned task. Delegate readiness to planner and discovery of the project token document to researcher. Both must read_task through inherited Aycorn MCP. Researcher must use project_context and read_project_document. Wait for both completed results. Do not implement."
		if phase == "working" {
			roles = []string{"coder", "reviewer"}
			req.Intent, req.RepoPath = "implement", work
			req.Instruction = "Implement the assigned task through coder, then use a separate reviewer to verify proof.txt against the project document. Both must read_task and read_project_document through inherited Aycorn MCP. Wait for both completed results and report actual evidence. Do not delegate planning again or create other files."
		}
		t.Logf("Starting %s through the production Conductor harness", phase)
		result, runErr := h.Run(ctx, RunSpec{Request: req, TaskID: 1, JobID: 1, WorkDir: work})
		if result.SessionID != "" {
			id := result.SessionID
			defer func() { _ = client.call(ctx, "thread/archive", map[string]string{"threadId": id}, nil) }()
		}
		if runErr != nil {
			t.Fatalf("%s: %v; session=%s; output=%s", phase, runErr, result.SessionID, result.Output)
		}
		var decision struct{ Ready, Completed bool }
		if err := json.Unmarshal([]byte(result.Output), &decision); err != nil || (phase == "planning" && !decision.Ready) || (phase == "working" && !decision.Completed) {
			t.Fatalf("invalid %s decision: %s (%v)", phase, result.Output, err)
		}
		history := read(result.SessionID)
		rootCalls := history.mcpCalls()
		if !rootCalls["read_task"] || !rootCalls["project_context"] {
			t.Fatalf("root Conductor did not read its MCP context: %v", rootCalls)
		}
		children := map[string]bool{}
		for _, item := range history.items() {
			if item.Type == "collabAgentToolCall" && item.Tool == "spawnAgent" && item.Status == "completed" {
				for _, id := range item.ReceiverThreadIDs {
					children[id] = true
				}
			}
			if item.Type == "subAgentActivity" && item.AgentThreadID != "" {
				children[item.AgentThreadID] = true
			}
		}
		seen := map[string]bool{}
		for id := range children {
			child := read(id)
			defer func() { _ = client.call(ctx, "thread/archive", map[string]string{"threadId": id}, nil) }()
			calls := child.mcpCalls()
			if !calls["read_task"] || (child.AgentRole != "planner" && !calls["read_project_document"]) {
				t.Fatalf("child %s (%s) did not exercise inherited MCP: %v", id, child.AgentRole, calls)
			}
			seen[child.AgentRole] = true
			t.Logf("Verified %s child %s with MCP calls %v", child.AgentRole, id, calls)
		}
		for _, role := range roles {
			if !seen[role] {
				t.Fatalf("%s did not run the registered %s profile; observed=%v", phase, role, seen)
			}
		}
		if phase == "planning" {
			if _, err := os.Stat(filepath.Join(work, "proof.txt")); !os.IsNotExist(err) {
				t.Fatal("planning wrote the deliverable")
			}
		}
	}
	proof, err := os.ReadFile(filepath.Join(work, "proof.txt"))
	if err != nil || string(proof) != token+"\n" {
		t.Fatalf("document context did not reach implementation: %q %v", proof, err)
	}
	var stage int
	if err = db.QueryRow("SELECT stage FROM task WHERE id=1").Scan(&stage); err != nil || stage != 1 {
		t.Fatalf("agent bypassed controller-owned stage transitions: %d %v", stage, err)
	}
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
