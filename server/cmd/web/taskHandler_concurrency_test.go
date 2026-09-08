package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	_ "modernc.org/sqlite"
)

func transitionTestApp(t *testing.T) *app {
	t.Helper()
	dsn := fmt.Sprintf("file:task_handler_cas_%s_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", t.Name(), time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(5)
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE workflow (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE stage (id INTEGER PRIMARY KEY, workflow INTEGER NOT NULL REFERENCES workflow(id) ON DELETE CASCADE, name TEXT NOT NULL DEFAULT '', description TEXT, color TEXT NOT NULL DEFAULT 'gray', icon TEXT NOT NULL DEFAULT 'circle', position INTEGER NOT NULL DEFAULT 0, type TEXT NOT NULL DEFAULT 'todo', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE persona (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', system_prompt TEXT NOT NULL DEFAULT '[]', harness TEXT NOT NULL DEFAULT 'opencode', model TEXT NOT NULL DEFAULT 'opencode-go/muse-spark-1.2-contributor', agent TEXT NOT NULL DEFAULT '', allowed_tools TEXT NOT NULL DEFAULT '[]', timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE stage_persona (stage_id INTEGER PRIMARY KEY REFERENCES stage(id) ON DELETE CASCADE, persona_id INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE);`,
		`CREATE TABLE project (id INTEGER PRIMARY KEY, name TEXT, pinned BOOLEAN, workflow INTEGER REFERENCES workflow(id), defaultView TEXT, timeCreated TEXT, timeModified TEXT, repoPath TEXT);`,
		`CREATE TABLE checklist (id INTEGER PRIMARY KEY, project INTEGER REFERENCES project(id), name TEXT, description TEXT, timeCreated TEXT, timeModified TEXT, isDefault BOOLEAN);`,
		`CREATE TABLE task_type_category (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
		`CREATE TABLE task_type (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT 'square-check', color TEXT NOT NULL DEFAULT 'gray', isDefault INTEGER NOT NULL DEFAULT 0, category INTEGER REFERENCES task_type_category(id), timeCreated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), timeModified TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')));`,
		`CREATE TABLE task (id INTEGER PRIMARY KEY, checklist INTEGER REFERENCES checklist(id), stage INTEGER NOT NULL REFERENCES stage(id) ON DELETE RESTRICT, type INTEGER NOT NULL REFERENCES task_type(id), name TEXT DEFAULT '', body TEXT DEFAULT '[]', timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP, timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP, timePlannedStart TIMESTAMP, timePlannedEnd TIMESTAMP, hasTimePlannedStart BOOLEAN NOT NULL DEFAULT 0, hasTimePlannedEnd BOOLEAN NOT NULL DEFAULT 0, timeCompleted TIMESTAMP, assignee TEXT, priority TEXT);`,
		`CREATE TABLE agent_job (id INTEGER PRIMARY KEY AUTOINCREMENT, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE, persona INTEGER NOT NULL REFERENCES persona(id) ON DELETE CASCADE, status TEXT NOT NULL, fromStage INTEGER, toStage INTEGER, claimedAt TEXT, startedAt TEXT, finishedAt TEXT, attempts INTEGER NOT NULL DEFAULT 0, error TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), requestJson TEXT NOT NULL DEFAULT '{}', progress TEXT NOT NULL DEFAULT '');`,
		`CREATE TABLE agent_run (id INTEGER PRIMARY KEY AUTOINCREMENT, job INTEGER NOT NULL REFERENCES agent_job(id) ON DELETE CASCADE, output TEXT, summary TEXT, exitCode INTEGER, usageJson TEXT, createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')), artifactJson TEXT NOT NULL DEFAULT '{}' );`,
		`CREATE INDEX idx_agent_job_status ON agent_job(status, createdAt);`,
	}
	for i, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create table %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workflow (id, name) VALUES (1, 'wf')`); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage (id, workflow, name, color, icon, position, type) VALUES (10,1,'open','gray','circle',1,'open'), (20,1,'todo','gray','circle',2,'todo'), (30,1,'doing','gray','circle',3,'doing')`); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO persona (id, name, harness, model) VALUES (1,'coder','opencode','opencode-go/muse-spark-1.2-contributor')`); err != nil {
		t.Fatalf("persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stage_persona (stage_id, persona_id) VALUES (20,1)`); err != nil {
		t.Fatalf("stage_persona: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, name, pinned, workflow, repoPath, defaultView) VALUES (1,'p',0,1,'','')`); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checklist (id, project, name, isDefault) VALUES (1,1,'c',1)`); err != nil {
		t.Fatalf("checklist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_type_category (id, name) VALUES (1,'cat')`); err != nil {
		t.Fatalf("task_type_category: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task_type (id, name, category, isDefault) VALUES (1,'Task',1,1)`); err != nil {
		t.Fatalf("task_type: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO task (id, checklist, stage, name, priority, type) VALUES (1,1,10,'task1','Medium',1)`); err != nil {
		t.Fatalf("task: %v", err)
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
	return &app{
		taskService:     taskService,
		agentJobService: agentJobService,
		projectRepo:     projectRepo,
	}
}

func TestTransitionTaskStage_Handler_MapsErrStageConflictTo409(t *testing.T) {
	app := transitionTestApp(t)
	h := app.routes()

	// First transition: 10 -> 20 should succeed
	body := map[string]int{"fromStage": 10, "toStage": 20}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/task/1/transition", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first transition status = %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	var ok bool
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil {
		t.Fatalf("decode first: %v body=%s", err, rec.Body.String())
	}
	if !ok {
		t.Fatal("first transition ok = false; want true")
	}

	// Second transition with stale fromStage 10 -> 20 should be 409
	b2, _ := json.Marshal(map[string]int{"fromStage": 10, "toStage": 20})
	req2 := httptest.NewRequest("POST", "/api/task/1/transition", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("stale transition status = %d want 409 body=%s", rec2.Code, rec2.Body.String())
	}

	// Direct mapping: httpStatusForError(ErrStageConflict) == 409
	if got := httpStatusForError(services.ErrStageConflict); got != http.StatusConflict {
		t.Fatalf("httpStatusForError(ErrStageConflict) = %d; want 409", got)
	}

	// Negative: unbound conflict also maps to 409 even though no job was enqueued
	// Task is now at 20, so fromStage 10 mismatches again but routed via unbound path if toStage=30.
	b3, _ := json.Marshal(map[string]int{"fromStage": 10, "toStage": 30})
	req3 := httptest.NewRequest("POST", "/api/task/1/transition", bytes.NewReader(b3))
	req3.Header.Set("Content-Type", "application/json")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusConflict {
		t.Fatalf("stale unbound transition status = %d want 409 body=%s", rec3.Code, rec3.Body.String())
	}
	_ = fmt.Sprintf
}

func TestHttpStatusForError_ErrStageConflict_Is409(t *testing.T) {
	if got := httpStatusForError(services.ErrStageConflict); got != 409 {
		t.Fatalf("httpStatusForError(ErrStageConflict) = %d; want 409", got)
	}
	if got := httpStatusForError(fmt.Errorf("wrapped: %w", services.ErrStageConflict)); got != 409 {
		t.Fatalf("wrapped ErrStageConflict status = %d; want 409", got)
	}
}
