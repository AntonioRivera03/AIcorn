package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

// assertNotFound fails the test unless err is a repos.NotFoundError naming
// entity and id — the clean error every id lookup below should produce
// instead of a raw "sql: no rows in result set".
func assertNotFound(t *testing.T, err error, entity string, id int) {
	t.Helper()
	var nfe repos.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("error = %v; want a repos.NotFoundError", err)
	}
	if nfe.Entity != entity || nfe.ID != id {
		t.Fatalf("NotFoundError = %+v; want {%s %d}", nfe, entity, id)
	}
}

// newNotFoundTestDB seeds one workflow with two stages, two projects each
// with a default checklist, and a task in each project. Project 1 ("Mine")
// has both the default document task type and the built-in Chat type
// enabled; project 2 ("Other") only has the document type. Task 1 belongs to
// project 1, task 2 to project 2 — used to confirm an out-of-scope task reads
// identically to a missing one.
func newNotFoundTestDB(t *testing.T) (*services.TaskService, int, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	var docType, chatType int
	if err := db.QueryRow("SELECT id FROM task_type WHERE name='Task'").Scan(&docType); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM task_type WHERE name='Chat'").Scan(&chatType); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
INSERT INTO workflow(id,name) VALUES(1,'Workflow');
INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1),(2,1,'Doing','doing','gray','circle',2);
INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Mine',0),(2,1,'Other',0);
INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Mine',1),(2,2,'Other',1);
`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT OR IGNORE INTO project_task_type(project,task_type) VALUES(1,?),(1,?),(2,?)`, docType, chatType, docType); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,?,'Mine','Medium','[]'),(2,2,1,?,'Other','Medium','[]')`, docType, docType); err != nil {
		t.Fatal(err)
	}
	return &services.TaskService{TaskRepo: &repos.TaskRepo{DB: db}}, docType, chatType
}

func TestReadTaskNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	ts := &toolset{runProjectID: 1, taskService: taskService, converter: &markdown.Converter{}}
	ctx := context.Background()

	if _, _, err := ts.readTask(ctx, nil, ReadTaskInput{TaskID: 99999999}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}

	// Task 2 exists, but in another project — must look the same as missing.
	if _, _, err := ts.readTask(ctx, nil, ReadTaskInput{TaskID: 2}); err == nil {
		t.Fatal("expected an error for an out-of-scope task")
	} else {
		assertNotFound(t, err, "task", 2)
	}
}

func TestUpdateTaskNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	ts := &toolset{runProjectID: 1, taskService: taskService}
	ctx := context.Background()
	name := "New name"

	if _, _, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 99999999, Name: &name}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}

	checklistID := 99999999
	if _, _, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 1, ChecklistID: &checklistID}); err == nil {
		t.Fatal("expected an error for a missing checklist")
	} else {
		assertNotFound(t, err, "checklist", checklistID)
	}

	typeID := 99999999
	if _, _, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 1, TypeID: &typeID}); err == nil {
		t.Fatal("expected an error for a missing task type")
	} else {
		assertNotFound(t, err, "task type", typeID)
	}
}

func TestMoveTaskStageNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	ts := &toolset{runProjectID: 1, taskService: taskService}
	ctx := context.Background()

	if _, _, err := ts.moveTaskStage(ctx, nil, MoveTaskStageInput{TaskID: 99999999, FromStage: 1, ToStage: 2}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}

	if _, _, err := ts.moveTaskStage(ctx, nil, MoveTaskStageInput{TaskID: 1, FromStage: 1, ToStage: 99999999}); err == nil {
		t.Fatal("expected an error for a missing destination stage")
	} else {
		assertNotFound(t, err, "stage", 99999999)
	}
}

// TestCreateTaskNotFound covers the unscoped, non-chat create_task path
// (CreateChecklistTask), which otherwise leans on FK constraints alone and
// fails with an opaque driver error for a bad agent-supplied id.
func TestCreateTaskNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	ts := &toolset{taskService: taskService}
	ctx := context.Background()

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 99999999, StageID: 1, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing checklist")
	} else {
		assertNotFound(t, err, "checklist", 99999999)
	}

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 99999999, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing stage")
	} else {
		assertNotFound(t, err, "stage", 99999999)
	}

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 1, TypeID: 99999999, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing task type")
	} else {
		assertNotFound(t, err, "task type", 99999999)
	}
}

// TestCreateTaskFromChatNotFound covers the chat-scoped create_task path
// (CreateFromChat), the other internal branch behind the same MCP tool.
func TestCreateTaskFromChatNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	if _, err := taskService.TaskRepo.DB.Exec(`
INSERT INTO project_chat(id,project) VALUES(1,1);
INSERT INTO project_chat_turn(id,conversation,clientKey,message,requestJson,status) VALUES(1,1,'a','Create a task','{}','running');`); err != nil {
		t.Fatal(err)
	}
	ts := &toolset{runProjectID: 1, runChatTurnID: 1, taskService: taskService}
	ctx := context.Background()

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 99999999, StageID: 1, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing checklist")
	} else {
		assertNotFound(t, err, "checklist", 99999999)
	}

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 99999999, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing stage")
	} else {
		assertNotFound(t, err, "stage", 99999999)
	}

	if _, _, err := ts.createTask(ctx, nil, CreateTaskInput{ChecklistID: 1, StageID: 1, TypeID: 99999999, Name: "x"}); err == nil {
		t.Fatal("expected an error for a missing task type")
	} else {
		assertNotFound(t, err, "task type", 99999999)
	}
}

func TestTaskLinksNotFound(t *testing.T) {
	taskService, _, _ := newNotFoundTestDB(t)
	ts := &toolset{runProjectID: 1, taskService: taskService}
	ctx := context.Background()

	if _, _, err := ts.listTaskLinks(ctx, nil, ReadTaskInput{TaskID: 99999999}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}
	if _, _, err := ts.addTaskLink(ctx, nil, AddTaskLinkInput{TaskID: 99999999, URL: "https://github.com/owner/repo/pull/1"}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}
	if _, _, err := ts.removeTaskLink(ctx, nil, RemoveTaskLinkInput{TaskID: 99999999, LinkID: 1, Revision: 1}); err == nil {
		t.Fatal("expected an error for a missing task")
	} else {
		assertNotFound(t, err, "task", 99999999)
	}

	// Task 2 exists, but in another project — must look the same as missing.
	if _, _, err := ts.listTaskLinks(ctx, nil, ReadTaskInput{TaskID: 2}); err == nil {
		t.Fatal("expected an error for an out-of-scope task")
	} else {
		assertNotFound(t, err, "task", 2)
	}
}
