package main

import (
	"context"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"path/filepath"
	"strings"
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
		`INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Mine',0),(2,1,'Other',0)`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Mine',0),(2,2,'Other',0)`,
		`INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Mine','Medium',''),(2,1,1,1,'Related','Medium',''),(3,2,1,1,'Other','Medium','')`,
		`INSERT INTO project_document(id,project,title,body) VALUES(1,1,'Requirements','[{"type":"p","children":[{"text":"Use a purple acorn."}]}]'),(2,2,'Private','[]')`,
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
	if len(list.Tools) != 4 {
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
		if tool.Name == "read_project_document" {
			arguments["documentId"] = 1
		}
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: arguments})
		if err != nil || result.IsError {
			raw, _ := json.Marshal(result)
			t.Fatalf("MCP %s call: %s %v", tool.Name, raw, err)
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

		if tool.Name == "project_context" && (!strings.Contains(string(raw), "Requirements") || strings.Contains(string(raw), "Private")) {
			t.Fatalf("wrong project context: %s", raw)
		}
		if tool.Name == "read_project_document" && !strings.Contains(string(raw), "purple acorn") {
			t.Fatalf("document notes not returned: %s", raw)
		}
		if tool.Name != "read_task" && tool.Name != "search_tasks" && tool.Name != "project_context" && tool.Name != "read_project_document" {
			t.Fatalf("write tool exposed: %s", tool.Name)
		}
	}
	if _, _, err = tools.readProjectDocument(ctx, nil, ReadDocumentInput{DocumentID: 2}); err == nil {
		t.Fatal("read another project's document")
	}
	if _, err = db.Exec("UPDATE task SET checklist=2 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = tools.projectContext(ctx, nil, NoInput{}); err == nil {
		t.Fatal("stale task binding retained project context access")
	}
	if _, _, err = tools.readProjectDocument(ctx, nil, ReadDocumentInput{DocumentID: 1}); err == nil {
		t.Fatal("stale task binding retained document access")
	}
}
