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
		Harness:      models.PersonaHarnessCodex,
		Model:        models.PersonaModelDefault,
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

func TestPersonaAPIOnlyModelsAreEditable(t *testing.T) {
	a, db := personaTestApp(t)
	request := personaRequester(t, a.routes())
	response := request(http.MethodGet, "/api/persona", nil)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var agents []models.Persona
	if err := json.Unmarshal(response.Body.Bytes(), &agents); err != nil {
		t.Fatal(err)
	}
	roles := map[string]bool{}
	for _, agent := range agents {
		if agent.BuiltinRole == "" {
			continue
		}
		roles[agent.BuiltinRole] = true
		if agent.Instructions == "" || agent.InstructionPath == "" || len(agent.Skills) != 1 || agent.Skills[0].Content == "" {
			t.Fatalf("missing read-only docs: %+v", agent)
		}
		path := "/api/persona/" + jsonNumber(agent.ID)
		response = request(http.MethodPut, path, map[string]string{"Model": "gpt-5.5"})
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		updated := decodePersona(t, request(http.MethodGet, path, nil))
		if updated.Model != "gpt-5.5" || updated.Instructions != agent.Instructions || updated.Name != agent.Name {
			t.Fatal("model update changed definition", updated)
		}
		for _, field := range []string{"Name", "SystemPrompt", "Harness", "Agent", "AllowedTools", "BuiltinRole", "Instructions"} {
			response = request(http.MethodPut, path, map[string]any{"Model": "gpt-5.6-sol", field: "replace"})
			if response.Code != 400 {
				t.Fatalf("accepted fixed field %s: %d", field, response.Code)
			}
		}
		response = request(http.MethodPut, path, map[string]string{"Model": "invalid"})
		if response.Code != 400 {
			t.Fatal("accepted invalid model", response.Code)
		}
		response = request(http.MethodDelete, path, nil)
		if response.Code != 403 {
			t.Fatal("allowed agent deletion", response.Code)
		}
	}
	for _, role := range []string{"conductor", "planner", "researcher", "coder", "reviewer", "chatter"} {
		if !roles[role] {
			t.Fatal("missing role", role)
		}
	}
	for _, op := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/persona", testPersona("New")},
		{"POST", "/api/persona/bulk", []models.Persona{testPersona("New")}},
		{"PUT", "/api/persona/bulk", agents},
		{"POST", "/api/persona/bulk/delete", []int{agents[0].ID}},
	} {
		response = request(op.method, op.path, op.body)
		if response.Code != 403 {
			t.Fatalf("allowed fixed agent mutation %s %s: %d", op.method, op.path, response.Code)
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM persona WHERE builtin_role<>''").Scan(&count); err != nil || count != 6 {
		t.Fatal(count, err)
	}
}

func TestLegacyPersonaModelUpdatePreservesSavedDefinition(t *testing.T) {
	a, db := personaTestApp(t)
	original := testPersona("Existing custom agent")
	p, err := a.personaRepo.Create(&original)
	if err != nil {
		t.Fatal(err)
	}
	request := personaRequester(t, a.routes())
	response := request("PUT", "/api/persona/"+jsonNumber(p.ID), map[string]string{"Model": "gpt-5.5"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var name, prompt, model string
	if err = db.QueryRow("SELECT name,system_prompt,model FROM persona WHERE id=?", p.ID).Scan(&name, &prompt, &model); err != nil || name != p.Name || prompt != original.SystemPrompt || model != "gpt-5.5" {
		t.Fatal(name, prompt, model, err)
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
