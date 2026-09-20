package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestConductorReservationBlocksAllCompetingAI(t *testing.T) {
	a := conductorTestApp(t)
	if result, err := a.conductorService.Repo.Manage(1, []int{1}, "send"); err != nil || result.Success != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("POST", "/api/ai/tasks/1/runs", strings.NewReader(`{"intent":"ask"}`)))
	if r.Code != 409 || !strings.Contains(r.Body.String(), "Conductor") {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	name := "Another agent's edit"
	if _, err := a.taskService.TaskRepo.UpdateByAgent(1, 1, 0, repos.AgentTaskPatch{Name: &name}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := a.taskService.TaskRepo.MoveByAgent(1, 1, 0, 2, 3); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := a.aiService.Jobs.DB.Exec("UPDATE task SET type=(SELECT id FROM task_type WHERE viewMode='chat' LIMIT 1) WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.aiService.StartChat(context.Background(), 1, services.ChatInput{Key: "reserved", Message: "Start", Mode: "ask"}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	// Legacy enqueue entry points are also protected by the database trigger.
	if _, err := a.agentJobService.Enqueue(1, 1, nil, nil); err == nil {
		t.Fatal("legacy queue stole Conductor's task")
	}
	r = httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("GET", "/api/task-ownership/project/1", nil))
	var owners []taskownership.Owner
	if err := json.Unmarshal(r.Body.Bytes(), &owners); err != nil || len(owners) != 1 || owners[0].Kind != "conductor" {
		t.Fatalf("%s %v", r.Body.String(), err)
	}
	if _, err := a.taskService.UpdateTaskBody(1, models.EmptyBody); err != nil {
		t.Fatal("human edit blocked", err)
	}
	if _, err := a.conductorService.Repo.Manage(1, []int{1}, "release"); err != nil {
		t.Fatal(err)
	}
	job, err := a.aiService.Start(context.Background(), 1, services.AIRunInput{Intent: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.taskService.TaskRepo.UpdateByAgent(1, 1, 0, repos.AgentTaskPatch{Name: &name}); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := a.aiService.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.taskService.TaskRepo.UpdateByAgent(1, 1, 0, repos.AgentTaskPatch{Name: &name}); err != nil {
		t.Fatal("canceled task stayed reserved", err)
	}
}

func TestManualRunAndConductorCannotBothClaimTask(t *testing.T) {
	a := conductorTestApp(t)
	for i := 0; i < 12; i++ {
		task := 100 + i
		if _, err := a.aiService.Jobs.DB.Exec("INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(?,1,2,1,'Race','Medium','')", task); err != nil {
			t.Fatal(err)
		}
		var result models.BulkResult
		var job *models.AgentJob
		var conductorErr, agentErr error
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			result, conductorErr = a.conductorService.Repo.Manage(1, []int{task}, "send")
		}()
		go func() {
			defer wg.Done()
			<-start
			job, agentErr = a.aiService.Jobs.EnqueueAI(task, 0, models.AIRunRequest{ProjectID: 1, Intent: "ask"})
		}()
		close(start)
		wg.Wait()
		if conductorErr != nil {
			t.Fatal(conductorErr)
		}
		if result.Success == 1 {
			if !errors.Is(agentErr, taskownership.ErrBusy) {
				t.Fatalf("both acquired task: %+v %v", job, agentErr)
			}
		} else if agentErr != nil || job == nil || result.Skipped != 1 {
			t.Fatalf("no winner: %+v %+v %v", result, job, agentErr)
		}
	}
}

func TestAgentTaskPatchIsAtomicAndProjectScoped(t *testing.T) {
	a := conductorTestApp(t)
	name, body := "New title", `[{"type":"p","children":[{"text":"Changed"}]}]`
	if _, err := a.taskService.TaskRepo.UpdateByAgent(1, 999, 0, repos.AgentTaskPatch{Name: &name}); err == nil {
		t.Fatal("cross-project edit")
	}
	invalidType := 999
	if _, err := a.taskService.TaskRepo.UpdateByAgent(1, 1, 0, repos.AgentTaskPatch{Name: &name, Body: &body, TypeID: &invalidType}); err == nil {
		t.Fatal("invalid patch accepted")
	}
	before, err := a.taskService.GetTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Name == name || before.Body == body {
		t.Fatal("partial patch committed")
	}
	if ok, err := a.taskService.TaskRepo.UpdateByAgent(1, 1, 0, repos.AgentTaskPatch{Name: &name}); err != nil || !ok {
		t.Fatal(err)
	}
	after, err := a.taskService.GetTask(1)
	if err != nil || after.Body != before.Body || after.Stage != before.Stage {
		t.Fatalf("unrelated fields changed: %+v %v", after, err)
	}
	if _, err := a.taskService.TaskRepo.MoveByAgent(1, 1, 0, 2, 6); err == nil {
		t.Fatal("moved to another workflow")
	}
	if ok, err := a.taskService.TaskRepo.MoveByAgent(1, 1, 0, 3, 4); err != nil || ok {
		t.Fatalf("stale stage accepted: %t %v", ok, err)
	}
	if ok, err := a.taskService.TaskRepo.MoveByAgent(1, 1, 0, 2, 3); err != nil || !ok {
		t.Fatal(err)
	}
}
