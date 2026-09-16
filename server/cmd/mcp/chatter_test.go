package main

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"path/filepath"
	"testing"
)

func TestChatterScopedMutationsRespectOwnersAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO workflow(id,name) VALUES(1,'W');
 INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1),(2,1,'Working','doing','gray','circle',2);
 INSERT INTO project(id,workflow,name) VALUES(1,1,'Mine'),(2,1,'Other');
 INSERT INTO checklist(id,project,name) VALUES(1,1,'Mine'),(2,2,'Other');
 INSERT OR IGNORE INTO project_task_type(project,task_type) VALUES(1,1),(2,1);
 INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Mine','Medium',''),(2,2,1,1,'Other','Medium','');
 INSERT INTO project_chat(id,project) VALUES(1,1);
 INSERT INTO project_chat_turn(id,conversation,clientKey,message,requestJson,status) VALUES(1,1,'a','Create tasks','{}','running');`); err != nil {
		t.Fatal(err)
	}
	ts := &toolset{runProjectID: 1, runChatTurnID: 1, converter: &markdown.Converter{}, taskService: &services.TaskService{TaskRepo: &repos.TaskRepo{DB: db}}}
	ctx := context.Background()
	_, created, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 1, Name: "Created in chat"})
	if err != nil || created.Priority != "Medium" {
		t.Fatalf("%+v %v", created, err)
	}
	if _, _, err = ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 2, StageID: 1, Name: "Out of scope"}); err == nil {
		t.Fatal("created outside project")
	}
	title := "Updated"
	if _, out, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: created.ID, Name: &title}); err != nil || !out.Ok {
		t.Fatal(err)
	}
	if _, out, err := ts.moveTaskStage(ctx, nil, MoveTaskStageInput{TaskID: created.ID, FromStage: 1, ToStage: 2}); err != nil || !out.Ok {
		t.Fatal(err)
	}
	if _, _, err = ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 2, Name: &title}); err == nil {
		t.Fatal("edited another project")
	}
	db.Exec("INSERT INTO agent_job(task,status) VALUES(?,'running')", created.ID)
	if _, _, err = ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: created.ID, Name: &title}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, _, err = ts.addTaskLink(ctx, nil, AddTaskLinkInput{TaskID: created.ID, URL: "https://github.com/a/b/pull/1"}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	db.Exec("UPDATE project_chat_turn SET status='canceling' WHERE id=1")
	if _, _, err = ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 1, Name: &title}); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal(err)
	}
	if _, _, err = ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 1, Name: "Too late"}); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal(err)
	}
	if _, _, err = ts.addTaskLink(ctx, nil, AddTaskLinkInput{TaskID: 1, URL: "https://github.com/a/b/pull/1"}); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal(err)
	}
}
func TestChatterToolCatalogIsProjectScoped(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "chatter", Version: "test"}, nil)
	(&toolset{runProjectID: 1, runChatTurnID: 1}).register(server)
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
	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"project_context", "read_project_document", "request_task_work", "create_task", "update_task", "move_task_stage", "read_task", "search_tasks", "add_task_link"} {
		if !names[name] {
			t.Fatal("missing", name)
		}
	}
	if names["list_projects"] || names["list_workflow_stages"] {
		t.Fatal("unscoped discovery exposed")
	}
}
