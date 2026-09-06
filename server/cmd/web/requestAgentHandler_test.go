package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func requestAgentTestApp(t *testing.T, repoPath string) (*app, *services.TaskService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := appdb.Migrate(db, dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'w')`); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, color, icon, position, type) VALUES (1,1,'Open','gray','circle',1,'open'), (2,1,'Plan','gray','circle',2,'todo')`); err != nil {
		t.Fatalf("stage: %v", err)
	}
	projRepoPath := repoPath
	if projRepoPath == "" && repoPath == "__valid_git__" {
		// create temp git repo
		dir := t.TempDir()
		if err := exec.Command("git", "init", dir).Run(); err != nil {
			t.Fatalf("git init: %v", err)
		}
		projRepoPath = dir
	} else if repoPath == "__valid_git__" {
		dir := t.TempDir()
		if err := exec.Command("git", "init", dir).Run(); err != nil {
			t.Fatalf("git init: %v", err)
		}
		projRepoPath = dir
	}
	if _, err := db.Exec(`INSERT INTO project (id, name, pinned, workflow, repoPath, defaultView) VALUES (1,'p',0,1, ?, '')`, projRepoPath); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1,1,'c',1)`); err != nil {
		t.Fatalf("checklist: %v", err)
	}
	var ct int
	db.QueryRow(`SELECT COUNT(*) FROM task_type`).Scan(&ct)
	if ct == 0 {
		if _, err := db.Exec(`INSERT INTO task_type_category (id, name) VALUES (1,'cat')`); err != nil {
		}
		if _, err := db.Exec(`INSERT INTO task_type (id, name, category, isDefault) VALUES (1,'Dev',1,1)`); err != nil {
			t.Fatalf("task_type: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, name, priority, type, assignee) VALUES (1,1,2,'t','Medium',1,'p')`); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1,'p','opencode','opencode-go/muse-spark-1.2-contributor')`); err != nil {
		t.Fatalf("persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (2,1)`); err != nil {
		t.Fatalf("stage_persona: %v", err)
	}
	taskRepo := &repos.TaskRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	projectRepo := &repos.ProjectRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	taskTypeRepo := &repos.TaskTypeRepo{DB: db}
	agentJobService := &services.AgentJobService{JobRepo: agentJobRepo, RunRepo: agentRunRepo, PersonaRepo: personaRepo, TaskRepo: taskRepo}
	taskService := &services.TaskService{
		TaskRepo:         taskRepo,
		TaskTypeRepo:     taskTypeRepo,
		AgentJobService:  agentJobService,
		StagePersonaRepo: stagePersonaRepo,
		ProjectRepo:      projectRepo,
		PersonaRepo:      personaRepo,
	}
	app := &app{
		taskService:     taskService,
		agentJobService: agentJobService,
		projectRepo:     projectRepo,
	}
	return app, taskService
}

func TestLegacyRequestAgentDoesNotExecute(t *testing.T) {
	app, _ := requestAgentTestApp(t, "")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/task/1/request-agent", nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("legacy endpoint status: %d", rec.Code)
	}
	jobs, err := app.agentJobService.FindByTask(1)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("legacy request queued work: %v %v", jobs, err)
	}
}
