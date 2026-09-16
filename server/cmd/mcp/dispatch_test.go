package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

func TestDispatcherExposesOnlyItsFiveTools(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "conductor", Version: "test"}, nil)
	(&toolset{runDispatchID: 1, runProjectID: 1}).register(server)
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
	want := map[string]bool{"list_conductor_tasks": true, "start_conductor_task": true, "defer_conductor_task": true, "project_context": true, "read_project_document": true}
	if len(listed.Tools) != len(want) {
		t.Fatalf("unexpected catalog: %+v", listed.Tools)
	}
	for _, tool := range listed.Tools {
		if !want[tool.Name] {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
	}
}

func TestDispatcherScopeRevokedAfterPauseOrCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dispatch.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO workflow(id,name) VALUES(1,'W');
 INSERT INTO project(id,workflow,name) VALUES(1,1,'Mine'),(2,1,'Other');
 INSERT INTO conductor_project(project,enabled,settings) VALUES(1,1,'{}');
 INSERT INTO conductor_dispatch(id,project,status,requestJson) VALUES(1,1,'running','{}');`); err != nil {
		t.Fatal(err)
	}
	repo := &repos.ConductorRepo{DB: db}
	ts := &toolset{runProjectID: 1, runDispatchID: 1, conductorService: &services.ConductorService{Repo: repo}, taskService: &services.TaskService{TaskRepo: &repos.TaskRepo{DB: db}}}
	ctx := context.Background()
	if _, _, err = ts.startConductorTask(ctx, nil, StartConductorTaskInput{ProjectID: 2, TaskID: 1, Role: "coder"}); err == nil {
		t.Fatal("cross-project start accepted")
	}
	for _, q := range []string{`UPDATE conductor_project SET enabled=0 WHERE project=1`, `UPDATE conductor_project SET enabled=1; UPDATE conductor_dispatch SET status='completed' WHERE id=1`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
		if _, _, err = ts.listConductorTasks(ctx, nil, NoInput{}); !errors.Is(err, taskownership.ErrNotOwner) {
			t.Fatalf("retained list access: %v", err)
		}
		if _, _, err = ts.startConductorTask(ctx, nil, StartConductorTaskInput{ProjectID: 1, TaskID: 1, Role: "coder"}); !errors.Is(err, taskownership.ErrNotOwner) {
			t.Fatalf("retained start access: %v", err)
		}
		if _, _, err = ts.deferConductorTask(ctx, nil, DeferConductorTaskInput{ProjectID: 1, TaskID: 1, Reason: "Blocked"}); !errors.Is(err, taskownership.ErrNotOwner) {
			t.Fatalf("retained defer access: %v", err)
		}
	}
}
