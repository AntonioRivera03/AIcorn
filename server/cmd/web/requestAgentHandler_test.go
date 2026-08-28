package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
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
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, name, priority, type, assignee) VALUES (1,1,2,'t','Medium',1,'')`); err != nil {
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
	}
	app := &app{
		taskService:     taskService,
		agentJobService: agentJobService,
		projectRepo:     projectRepo,
	}
	return app, taskService
}

func TestRequestAgent_Success(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	app, _ := requestAgentTestApp(t, dir)
	handler := app.routes()
	req := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]models.AgentJob
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	job, ok := resp["job"]
	if !ok {
		t.Fatalf("response missing job key: %s", rec.Body.String())
	}
	if job.Task != 1 || job.Persona != 1 || job.Status != models.AgentJobStatusPending {
		t.Fatalf("job mismatch: %+v", job)
	}
	if job.ToStage == nil || *job.ToStage != 2 {
		t.Fatalf("toStage = %v want 2", job.ToStage)
	}
	// Ensure task stage not changed
	var stage int
	app.taskService.TaskRepo.DB.QueryRow(`SELECT stage FROM task WHERE id = 1`).Scan(&stage)
	if stage != 2 {
		t.Fatalf("task stage after request-agent = %d want 2 unchanged", stage)
	}
}

func TestRequestAgent_NoPersona409(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	app, _ := requestAgentTestApp(t, dir)
	// remove persona binding
	if _, err := app.taskService.StagePersonaRepo.DB.Exec(`DELETE FROM stage_persona WHERE stage_id = 2`); err != nil {
		t.Fatalf("delete binding: %v", err)
	}
	handler := app.routes()
	req := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d want 409 body=%s", rec.Code, rec.Body.String())
	}
	if want := "no persona bound to this stage"; !contains(rec.Body.String(), want) {
		t.Fatalf("body %q should contain %q", rec.Body.String(), want)
	}
}

func TestRequestAgent_EmptyRepoPath400(t *testing.T) {
	app, _ := requestAgentTestApp(t, "")
	handler := app.routes()
	req := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400 body=%s", rec.Code, rec.Body.String())
	}
	if want := "project has no repo folder linked"; !contains(rec.Body.String(), want) {
		t.Fatalf("body %q should contain %q", rec.Body.String(), want)
	}
}

func TestRequestAgent_InvalidGitRepo400(t *testing.T) {
	dir := t.TempDir() // not a git repo
	app, _ := requestAgentTestApp(t, dir)
	handler := app.routes()
	req := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400 body=%s", rec.Code, rec.Body.String())
	}
	if want := "linked repo folder is not a valid git repository"; !contains(rec.Body.String(), want) {
		t.Fatalf("body %q should contain %q", rec.Body.String(), want)
	}
}

func TestRequestAgent_IdempotentSecondCall(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	app, _ := requestAgentTestApp(t, dir)
	handler := app.routes()
	req1 := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	var first map[string]models.AgentJob
	if err := json.Unmarshal(rec1.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	req2 := httptest.NewRequest("POST", "/api/task/1/request-agent", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second call status=%d want 409 body=%s", rec2.Code, rec2.Body.String())
	}
	if want := "already pending"; !contains(rec2.Body.String(), want) {
		t.Fatalf("second body %q should contain %q", rec2.Body.String(), want)
	}
	// ensure only one pending job exists
	var cnt int
	app.taskService.TaskRepo.DB.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE task=1 AND status='pending'`).Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("pending count=%d want 1", cnt)
	}
	_ = first
}

func TestRequestAgent_NotFound404(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	app, _ := requestAgentTestApp(t, dir)
	handler := app.routes()
	req := httptest.NewRequest("POST", "/api/task/999/request-agent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", rec.Code, rec.Body.String())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
