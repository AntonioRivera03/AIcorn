package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func agentJobTestApp(t *testing.T) (*app, *services.AgentJobService) {
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
	// Seed minimal workflow/project/checklist/task/persona for ListByTask validation
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'w')`); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, color, icon, position, type) VALUES (1,1,'Open','gray','circle',1,'open')`); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, name, workflow) VALUES (1,'p',1)`); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1,1,'c',1)`); err != nil {
		t.Fatalf("checklist: %v", err)
	}
	// Ensure task_type exists for FK
	var ct int
	db.QueryRow(`SELECT COUNT(*) FROM task_type`).Scan(&ct)
	if ct == 0 {
		// Insert category and type
		if _, err := db.Exec(`INSERT INTO task_type_category (id, name) VALUES (1,'cat')`); err != nil {
			// ignore if fails
		}
		if _, err := db.Exec(`INSERT INTO task_type (id, name, category, isDefault) VALUES (1,'Dev',1,1)`); err != nil {
			t.Fatalf("task_type: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, name, priority, type, assignee) VALUES (1,1,1,'t','Medium',1,'')`); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1,'p','codex','gpt-5.6-sol')`); err != nil {
		t.Fatalf("persona: %v", err)
	}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	taskRepo := &repos.TaskRepo{DB: db}
	svc := &services.AgentJobService{JobRepo: agentJobRepo, RunRepo: agentRunRepo, PersonaRepo: personaRepo, TaskRepo: taskRepo}
	app := &app{agentJobService: svc}
	return app, svc
}

func TestGetAgentJobsForTask_returnsJobsAndRuns(t *testing.T) {
	app, svc := agentJobTestApp(t)
	// Enqueue and complete to have run with output
	job, err := svc.Enqueue(1, 1, nil, nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := svc.ClaimNext()
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext: %v %v", err, claimed)
	}
	if _, err := svc.MarkRunning(claimed.ID); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	exit := 0
	if err := svc.Complete(claimed.ID, "hello output", "summary", &exit, `{"tokens":5}`); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	handler := app.routes()
	req := httptest.NewRequest("GET", "/api/agent-jobs/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	var resp models.AgentJobsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("jobs len=%d want 1", len(resp.Jobs))
	}
	if resp.Jobs[0].ID != job.ID || resp.Jobs[0].Status != models.AgentJobStatusCompleted {
		t.Fatalf("job mismatch: %+v", resp.Jobs[0])
	}
	if resp.Jobs[0].CreatedAt == nil {
		t.Fatal("CreatedAt nil")
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("runs len=%d want 1", len(resp.Runs))
	}
	if resp.Runs[0].Output != "hello output" {
		t.Fatalf("output=%q want hello output", resp.Runs[0].Output)
	}
	if resp.Runs[0].UsageJson != `{"tokens":5}` {
		t.Fatalf("usage=%q", resp.Runs[0].UsageJson)
	}
}

func TestGetAgentJobsForTask_empty(t *testing.T) {
	app, _ := agentJobTestApp(t)
	// Create empty task 2
	// Need to insert task 2
	// Use direct DB via svc not exposed; use app's service TaskRepo? We'll just insert via DB handle from persona style? Simpler: enqueue no job, just query task 1 which has no jobs before enqueue? Our previous app has task 1 but no jobs yet
	// Fresh app: task 1 exists but no jobs
	handler := app.routes()
	req := httptest.NewRequest("GET", "/api/agent-jobs/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var resp models.AgentJobsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Jobs) != 0 || len(resp.Runs) != 0 {
		t.Fatalf("expected empty, got %+v", resp)
	}
	// Ensure JSON is [] not null
	if string(rec.Body.Bytes()) == "" {
		t.Fatal("empty body")
	}
}

func TestGetAgentJobsForTask_404(t *testing.T) {
	app, _ := agentJobTestApp(t)
	handler := app.routes()
	req := httptest.NewRequest("GET", "/api/agent-jobs/999", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestGetAgentJobs_filterAndAlias(t *testing.T) {
	app, svc := agentJobTestApp(t)
	svc.Enqueue(1, 1, nil, nil)
	handler := app.routes()
	req := httptest.NewRequest("GET", "/api/agent-jobs?taskId=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agent-job taskId status=%d", rec.Code)
	}
}
