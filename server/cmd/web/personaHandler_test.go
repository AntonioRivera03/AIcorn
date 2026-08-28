package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	_ "modernc.org/sqlite"
)

func personaTestApp(t *testing.T) (*app, *sql.DB) {
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

	repo := &repos.PersonaRepo{DB: db}
	return &app{personaRepo: repo, personaService: &services.PersonaService{PersonaRepo: repo}}, db
}

func personaRequester(t *testing.T, handler http.Handler) func(string, string, any) *httptest.ResponseRecorder {
	t.Helper()
	return func(method, path string, body any) *httptest.ResponseRecorder {
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatalf("encode request body: %v", err)
			}
		}
		req := httptest.NewRequest(method, path, &payload)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}
}

func testPersona(name string) models.Persona {
	return models.Persona{
		Name:         name,
		SystemPrompt: "Research carefully.",
		Harness:      models.PersonaHarnessOpencode,
		Model:        models.PersonaModelSonnet,
		AllowedTools: []string{"read_task", "search_tasks"},
	}
}

func decodePersona(t *testing.T, recorder *httptest.ResponseRecorder) models.Persona {
	t.Helper()

	var persona models.Persona
	if err := json.Unmarshal(recorder.Body.Bytes(), &persona); err != nil {
		t.Fatalf("decode persona response: %v", err)
	}
	return persona
}

func decodeBulkResult(t *testing.T, recorder *httptest.ResponseRecorder) models.BulkResult {
	t.Helper()

	var result models.BulkResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode bulk result: %v", err)
	}
	return result
}

func TestPersonaAPI_supports_full_lifecycle(t *testing.T) {
	// Given
	testApp, _ := personaTestApp(t)
	handler := testApp.routes()
	request := personaRequester(t, handler)

	// When
	createdResponse := request(http.MethodPost, "/api/persona", testPersona("Researcher"))

	// Then
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d; want 200: %s", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodePersona(t, createdResponse)
	if created.ID == 0 || created.Name != "Researcher" || len(created.AllowedTools) != 2 {
		t.Fatalf("created persona = %#v; want persisted fields", created)
	}

	// When
	getResponse := request(http.MethodGet, "/api/persona/"+jsonNumber(created.ID), nil)
	listResponse := request(http.MethodGet, "/api/persona", nil)

	// Then
	if getResponse.Code != http.StatusOK || listResponse.Code != http.StatusOK {
		t.Fatalf("get/list statuses = %d/%d; want 200/200", getResponse.Code, listResponse.Code)
	}
	var listed []models.Persona
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode persona list: %v", err)
	}
	// Migration 00013 seeds a Coder persona, so list contains at least the seeded one plus created.
	found := false
	for _, p := range listed {
		if p.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("listed personas = %#v; want created ID %d", listed, created.ID)
	}

	// When
	updated := created
	updated.Name = "Lead Researcher"
	updated.Model = models.PersonaModelOpus
	updateResponse := request(http.MethodPut, "/api/persona/"+jsonNumber(created.ID), updated)

	// Then
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d; want 200: %s", updateResponse.Code, updateResponse.Body.String())
	}
	readUpdated := decodePersona(t, request(http.MethodGet, "/api/persona/"+jsonNumber(created.ID), nil))
	if readUpdated.Name != updated.Name || readUpdated.Model != models.PersonaModelOpus {
		t.Fatalf("updated persona = %#v; want name/model update", readUpdated)
	}

	// When
	deleteResponse := request(http.MethodDelete, "/api/persona/"+jsonNumber(created.ID), nil)

	// Then
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d; want 200: %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	missingResponse := request(http.MethodGet, "/api/persona/"+jsonNumber(created.ID), nil)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("deleted persona status = %d; want 404", missingResponse.Code)
	}
}

func TestPersonaAPI_rejects_invalid_harness_and_model(t *testing.T) {
	// Given
	testApp, _ := personaTestApp(t)
	request := personaRequester(t, testApp.routes())
	persona := testPersona("Invalid")
	persona.Harness = "other"
	persona.Model = "future-model"

	// When
	response := request(http.MethodPost, "/api/persona", persona)

	// Then
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid persona status = %d; want 400: %s", response.Code, response.Body.String())
	}
}

func TestPersonaAPI_bulk_operations_report_counts_and_delete_bindings(t *testing.T) {
	// Given
	testApp, db := personaTestApp(t)
	handler := testApp.routes()
	request := personaRequester(t, handler)
	personas := []models.Persona{testPersona("One"), testPersona("Two")}

	// When
	createResponse := request(http.MethodPost, "/api/persona/bulk", personas)

	// Then
	if createResponse.Code != http.StatusOK {
		t.Fatalf("bulk create status = %d; want 200: %s", createResponse.Code, createResponse.Body.String())
	}
	if got := decodeBulkResult(t, createResponse); got != (models.BulkResult{Success: 2}) {
		t.Fatalf("bulk create result = %#v; want 2 successes", got)
	}

	var stored []models.Persona
	listResponse := request(http.MethodGet, "/api/persona", nil)
	if err := json.Unmarshal(listResponse.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode personas: %v", err)
	}
	var target models.Persona
	for _, p := range stored {
		if p.Name == "One" || p.Name == "Two" {
			target = p
			break
		}
	}
	if target.ID == 0 {
		t.Fatalf("stored personas = %#v; want One/Two present", stored)
	}
	updates := []models.Persona{target, testPersona("Missing")}
	updates[0].Name = "Updated"
	updates[1].ID = 999999

	updateResponse := request(http.MethodPut, "/api/persona/bulk", updates)

	if got := decodeBulkResult(t, updateResponse); got != (models.BulkResult{Success: 1, Skipped: 1}) {
		t.Fatalf("bulk update result = %#v; want 1 success and 1 skipped", got)
	}

	stageID := insertPersonaTestStage(t, db)
	if _, err := db.Exec("INSERT INTO stage_persona (stage_id, persona_id) VALUES (?, ?)", stageID, target.ID); err != nil {
		t.Fatalf("bind persona to stage: %v", err)
	}

	deleteResponse := request(http.MethodPost, "/api/persona/bulk/delete", []int{target.ID, 999999})

	// Then
	if got := decodeBulkResult(t, deleteResponse); got != (models.BulkResult{Success: 1, Skipped: 1}) {
		t.Fatalf("bulk delete result = %#v; want 1 success and 1 skipped", got)
	}
	var bindings int
	if err := db.QueryRow("SELECT COUNT(*) FROM stage_persona WHERE persona_id = ?", target.ID).Scan(&bindings); err != nil {
		t.Fatalf("count persona bindings: %v", err)
	}
	if bindings != 0 {
		t.Fatalf("binding count after persona deletion = %d; want 0", bindings)
	}
}

func TestMCPToolsAPI_returns_shared_tool_catalog(t *testing.T) {
	// Given
	testApp, _ := personaTestApp(t)
	request := personaRequester(t, testApp.routes())

	// When
	response := request(http.MethodGet, "/api/mcp/tools", nil)

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("MCP tools status = %d; want 200: %s", response.Code, response.Body.String())
	}
	var tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &tools); err != nil {
		t.Fatalf("decode MCP tools: %v", err)
	}
	if len(tools) == 0 || tools[0].Name == "" || tools[0].Description == "" {
		t.Fatalf("MCP tools = %#v; want named, described tools", tools)
	}
}

func insertPersonaTestStage(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	workflow, err := db.Exec("INSERT INTO workflow (name) VALUES ('Persona API Test')")
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	workflowID, err := workflow.LastInsertId()
	if err != nil {
		t.Fatalf("read workflow id: %v", err)
	}
	stage, err := db.Exec(`
		INSERT INTO stage (workflow, name, color, icon, position, type)
		VALUES (?, 'Open', 'gray', 'circle-dashed', 1, 'open')
	`, workflowID)
	if err != nil {
		t.Fatalf("insert stage: %v", err)
	}
	stageID, err := stage.LastInsertId()
	if err != nil {
		t.Fatalf("read stage id: %v", err)
	}
	return stageID
}

func jsonNumber(value int) string {
	return strconv.Itoa(value)
}
