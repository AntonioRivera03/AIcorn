package main

import (
	"context"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"path/filepath"
	"testing"
)

func TestLinkMCPScopesAndOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO workflow(id,name) VALUES(1,'Workflow');
INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1);
INSERT INTO project(id,workflow,name,pinned) VALUES(1,1,'Mine',0),(2,1,'Other',0);
INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Mine',1),(2,2,'Other',1);
INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(1,1,1,1,'Mine','Medium',''),(2,1,1,1,'Related','Medium',''),(3,2,1,1,'Other','Medium','');
INSERT INTO agent_job(id,task,status) VALUES(41,1,'running');`); err != nil {
		t.Fatal(err)
	}
	ts := &toolset{runTaskID: 1, runJobID: 41, runProjectID: 1, taskService: &services.TaskService{TaskRepo: &repos.TaskRepo{DB: db}}}
	ctx := context.Background()
	_, link, err := ts.addTaskLink(ctx, nil, AddTaskLinkInput{TaskID: 1, URL: "https://github.com/owner/repo/pull/1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []int{2, 3} {
		if _, _, err := ts.addTaskLink(ctx, nil, AddTaskLinkInput{TaskID: task, URL: "https://github.com/owner/repo/pull/1"}); err == nil {
			t.Fatalf("wrote task %d", task)
		}
	}
	if _, _, err := ts.listTaskLinks(ctx, nil, ReadTaskInput{TaskID: 3}); err == nil {
		t.Fatal("read another project")
	}
	if _, output, err := ts.listTaskLinks(ctx, nil, ReadTaskInput{TaskID: 1}); err != nil || len(output.Links) != 1 {
		t.Fatalf("%+v %v", output, err)
	}
	external := &toolset{taskService: ts.taskService}
	if _, _, err := external.removeTaskLink(ctx, nil, RemoveTaskLinkInput{TaskID: 1, LinkID: link.ID, Revision: link.Revision}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE agent_job SET status='completed' WHERE id=41"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ts.removeTaskLink(ctx, nil, RemoveTaskLinkInput{TaskID: 1, LinkID: link.ID, Revision: link.Revision}); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal(err)
	}
	if _, out, err := external.removeTaskLink(ctx, nil, RemoveTaskLinkInput{TaskID: 1, LinkID: link.ID, Revision: link.Revision}); err != nil || !out.Ok {
		t.Fatal(err)
	}
}
