package main

import (
	"context"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func TestConductorMCPReadsOnlyItsProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Workflow')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1)`,
		`INSERT INTO project(id,workflow,name) VALUES(1,1,'Mine'),(2,1,'Other')`,
		`INSERT INTO checklist(id,project,name) VALUES(1,1,'Mine'),(2,2,'Other')`,
		`INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Mine','Medium',''),(2,1,1,1,'Related','Medium',''),(3,2,1,1,'Other','Medium','')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	tools := &toolset{runTaskID: 1, runProjectID: 1, taskService: &services.TaskService{TaskRepo: &repos.TaskRepo{DB: db}}, converter: &markdown.Converter{}}
	ctx := context.Background()
	_, output, err := tools.searchTasks(ctx, nil, SearchTasksInput{ProjectIDs: []int{2}})
	if err != nil || len(output.Tasks) != 2 {
		t.Fatalf("search escaped scope: %+v %v", output, err)
	}
	for _, task := range output.Tasks {
		if task.ProjectID != 1 {
			t.Fatal("read other project")
		}
	}
	if _, _, err = tools.readTask(ctx, nil, ReadTaskInput{TaskID: 2}); err != nil {
		t.Fatal("could not read related task", err)
	}
	if _, _, err = tools.readTask(ctx, nil, ReadTaskInput{TaskID: 3}); err == nil {
		t.Fatal("read other project's task")
	}
	st, ct := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	tools.register(server)
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 2 {
		t.Fatal("unexpected tools exposed")
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatal("reads require read-only annotation")
		}
		arguments := map[string]any{}
		if tool.Name == "read_task" {
			arguments["taskId"] = 2
		}
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("MCP call: %+v %v", result, err)
		}
		raw, _ := json.Marshal(tool.OutputSchema)
		var schema jsonschema.Schema
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ = json.Marshal(result.StructuredContent)
		var value any
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err = resolved.Validate(value); err != nil {
			t.Fatalf("output violates declared MCP schema: %v", err)
		}

		if tool.Name != "read_task" && tool.Name != "search_tasks" {
			t.Fatalf("write tool exposed: %s", tool.Name)
		}
	}
}
