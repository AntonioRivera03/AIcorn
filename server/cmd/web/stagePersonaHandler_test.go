package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

type stagePersonaTestServer struct {
	server        *httptest.Server
	db            *sql.DB
	workflowID    int
	stageID       int
	firstPersona  int
	secondPersona int
}

func newStagePersonaTestServer(t *testing.T) *stagePersonaTestServer {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := appdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := appdb.Migrate(db, dbPath); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	workflowID := insertTestRow(t, db, "INSERT INTO workflow (name) VALUES ('Test workflow')")
	stageID := insertTestRow(t, db, `
		INSERT INTO stage (workflow, name, color, icon, position, type)
		VALUES (?, 'Ready', 'gray', 'circle', 1, 'open')
	`, workflowID)
	firstPersona := insertTestRow(t, db, `
		INSERT INTO persona (name, system_prompt, harness, model)
		VALUES ('Researcher', 'Research', 'claude-code', 'sonnet')
	`)
	secondPersona := insertTestRow(t, db, `
		INSERT INTO persona (name, system_prompt, harness, model)
		VALUES ('Implementer', 'Implement', 'claude-code', 'opus')
	`)

	stageRepo := &repos.StageRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	application := &app{
		stageService: &services.StageService{
			StageRepo:        stageRepo,
			StagePersonaRepo: stagePersonaRepo,
		},
		workflowService: &services.WorkflowService{
			WorkflowRepo: &repos.WorkflowRepo{DB: db},
			ProjectRepo:  &repos.ProjectRepo{DB: db},
			StageRepo:    stageRepo,
		},
	}
	server := httptest.NewServer(application.routes())
	t.Cleanup(server.Close)

	return &stagePersonaTestServer{
		server:        server,
		db:            db,
		workflowID:    workflowID,
		stageID:       stageID,
		firstPersona:  firstPersona,
		secondPersona: secondPersona,
	}
}

func insertTestRow(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	result, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read fixture id: %v", err)
	}
	return int(id)
}

func (fixture *stagePersonaTestServer) bind(t *testing.T, personaID int) *http.Response {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"personaId":%d}`, personaID))
	request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/stage/%d/persona", fixture.server.URL, fixture.stageID), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create bind request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatalf("bind persona: %v", err)
	}
	return response
}

func TestStagePersonaAPI_bind_hydratesStageAndWorkflowReads(t *testing.T) {
	fixture := newStagePersonaTestServer(t)

	response := fixture.bind(t, fixture.firstPersona)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bind status = %d; want %d", response.StatusCode, http.StatusOK)
	}

	stageResponse, err := fixture.server.Client().Get(fixture.server.URL + "/api/stage")
	if err != nil {
		t.Fatalf("get stages: %v", err)
	}
	defer stageResponse.Body.Close()
	var stages []models.Stage
	if err := json.NewDecoder(stageResponse.Body).Decode(&stages); err != nil {
		t.Fatalf("decode stages: %v", err)
	}
	if len(stages) != 1 || stages[0].Persona == nil || stages[0].Persona.ID != fixture.firstPersona {
		t.Fatalf("stage persona = %#v; want persona %d", stages[0].Persona, fixture.firstPersona)
	}

	workflowResponse, err := fixture.server.Client().Get(fmt.Sprintf("%s/api/workflow/%d", fixture.server.URL, fixture.workflowID))
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	defer workflowResponse.Body.Close()
	var workflow services.WorkflowSummary
	if err := json.NewDecoder(workflowResponse.Body).Decode(&workflow); err != nil {
		t.Fatalf("decode workflow: %v", err)
	}
	if len(workflow.Stages) != 1 || workflow.Stages[0].Persona == nil || workflow.Stages[0].Persona.Name != "Researcher" {
		t.Fatalf("workflow stage persona = %#v; want Researcher", workflow.Stages[0].Persona)
	}
}

func TestStagePersonaAPI_bind_replacesExistingPersona(t *testing.T) {
	fixture := newStagePersonaTestServer(t)
	first := fixture.bind(t, fixture.firstPersona)
	first.Body.Close()

	response := fixture.bind(t, fixture.secondPersona)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replace status = %d; want %d", response.StatusCode, http.StatusOK)
	}

	stage, err := (&repos.StageRepo{DB: fixture.db}).FindOne(fixture.stageID)
	if err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if stage.Persona == nil || stage.Persona.ID != fixture.secondPersona {
		t.Fatalf("stage persona = %#v; want persona %d", stage.Persona, fixture.secondPersona)
	}
}

func TestStageRepo_ByWorkflowForProject_hydratesPersona(t *testing.T) {
	fixture := newStagePersonaTestServer(t)
	response := fixture.bind(t, fixture.firstPersona)
	response.Body.Close()

	stages, err := (&repos.StageRepo{DB: fixture.db}).ByWorkflowForProject(fixture.workflowID, 999999)
	if err != nil {
		t.Fatalf("read project-scoped stages: %v", err)
	}
	if len(stages) != 1 || stages[0].Persona == nil || stages[0].Persona.ID != fixture.firstPersona {
		t.Fatalf("project-scoped stage persona = %#v; want persona %d", stages[0].Persona, fixture.firstPersona)
	}
}

func TestStagePersonaAPI_unbind_clearsPersona(t *testing.T) {
	fixture := newStagePersonaTestServer(t)
	bound := fixture.bind(t, fixture.firstPersona)
	bound.Body.Close()

	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/stage/%d/persona", fixture.server.URL, fixture.stageID), nil)
	if err != nil {
		t.Fatalf("create unbind request: %v", err)
	}
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatalf("unbind persona: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unbind status = %d; want %d", response.StatusCode, http.StatusOK)
	}

	stage, err := (&repos.StageRepo{DB: fixture.db}).FindOne(fixture.stageID)
	if err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if stage.Persona != nil {
		t.Fatalf("stage persona = %#v; want nil", stage.Persona)
	}
}

func TestStagePersonaAPI_bind_returnsNotFoundForUnknownPersona(t *testing.T) {
	fixture := newStagePersonaTestServer(t)

	response := fixture.bind(t, 999999)
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("bind status = %d; want %d", response.StatusCode, http.StatusNotFound)
	}
}
