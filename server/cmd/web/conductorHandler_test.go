package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func conductorTestApp(t *testing.T) *app {
	a := aiTestApp(t, "")
	db := a.aiService.Jobs.DB
	for _, q := range []string{
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(3,1,'Doing','doing','gray','circle',3),(4,1,'Review','todo','gray','circle',4),(5,1,'Done','done','gray','circle',5)`,
		`INSERT INTO workflow(id,name) VALUES(2,'Other')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(6,2,'Other','open','gray','circle',1)`,
		`UPDATE ai_settings SET model='gpt-5.6-sol'`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	a.conductorService = &services.ConductorService{Repo: &repos.ConductorRepo{DB: db}, AI: a.aiService, Runs: a.agentJobService.RunRepo}
	return a
}

func TestConductorSettingsAndBulkHTTP(t *testing.T) {
	a := conductorTestApp(t)
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		if r.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	url := "/api/project/1/settings/conductor"
	call("GET", url, "", 200)
	call("PUT", url, `{"conductorAgentId":1}`, 400)
	call("PUT", url, `{"taskAgentId":1}`, 400)
	call("PUT", url, `{"enabled":true}`, 400)
	call("PUT", url, `{"planningStage":2,"workingStage":3,"completionStage":4,"useRepository":false}`, 200)
	call("PUT", url, `{"enabled":true}`, 200)
	call("PUT", url, `{"completionStage":5}`, 400)
	call("PUT", url, `{"completionStage":6}`, 400)
	call("PUT", url, `{"workingStage":4}`, 400)
	call("PUT", url, `{"workerModels":["invalid model"]}`, 400)
	call("PUT", url, `{"workerModels":null}`, 400)
	call("PUT", url, `{"fullAccess":true}`, 400)
	call("PUT", url, `{"planningPrompt":"Custom planning rules"}`, 200)
	call("PUT", url, `{"workingPrompt":"Custom worker rules"}`, 200)
	r := call("GET", url, "", 200)
	var board services.ConductorBoard
	if err := json.Unmarshal(r.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	root, err := a.aiService.Presets.FindRole("conductor")
	if err != nil || root.ID != board.Settings.ConductorAgentID {
		t.Fatalf("missing default orchestrator: %+v %v", board, err)
	}
	if board.Settings.PlanningPrompt != "Custom planning rules" || board.Settings.WorkingPrompt != "Custom worker rules" {
		t.Fatal("field patches overwrote one another")
	}
	call("POST", "/api/project/1/conductor/bulk", `{"ids":[1,999],"action":"send"}`, 200)
	call("POST", "/api/project/1/conductor/bulk", `{"ids":[],"action":"send"}`, 400)
	call("POST", "/api/project/1/conductor/bulk", `{"ids":[1],"action":"delete"}`, 400)
	call("GET", "/api/project/999/settings/conductor", "", 404)
	call("PUT", url, `{"enabled":false}`, 200)
}

func TestTaskBodyRevisionProtectsConductorNotes(t *testing.T) {
	a := conductorTestApp(t)
	get := httptest.NewRecorder()
	a.routes().ServeHTTP(get, httptest.NewRequest("GET", "/api/task/body/1", nil))
	etag := get.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no revision")
	}
	newBody := `[{"type":"p","children":[{"text":"Conductor handoff"}]}]`
	if _, err := a.taskService.UpdateTaskBody(1, newBody); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", "/api/task/body/1", strings.NewReader(`[]`))
	req.Header.Set("If-Match", etag)
	response := httptest.NewRecorder()
	a.routes().ServeHTTP(response, req)
	if response.Code != 412 {
		t.Fatalf("stale save accepted: %d %s", response.Code, response.Body.String())
	}
	body, err := a.taskService.GetTaskBody(1)
	if err != nil || body != newBody {
		t.Fatal("summary overwritten")
	}
	req = httptest.NewRequest("PUT", "/api/task/body/1", strings.NewReader(newBody))
	req.Header.Set("If-Match", taskBodyETag(newBody))
	response = httptest.NewRecorder()
	a.routes().ServeHTTP(response, req)
	if response.Code != 200 || response.Header().Get("ETag") == "" {
		t.Fatal("current save rejected")
	}
}

func TestBodyRevisionAcceptsHistoricalEmptyDocuments(t *testing.T) {
	for _, body := range []string{"[]", "", `[{"type":"p","children":[{"text":""}]}]`} {
		t.Run(body, func(t *testing.T) {
			a := conductorTestApp(t)
			if _, err := a.taskService.UpdateTaskBody(1, body); err != nil {
				t.Fatal(err)
			}
			get := httptest.NewRecorder()
			a.routes().ServeHTTP(get, httptest.NewRequest("GET", "/api/task/body/1", nil))
			req := httptest.NewRequest("PUT", "/api/task/body/1", strings.NewReader(`[{"type":"p","children":[{"text":"New context"}]}]`))
			req.Header.Set("If-Match", get.Header().Get("ETag"))
			response := httptest.NewRecorder()
			a.routes().ServeHTTP(response, req)
			if response.Code != 200 {
				t.Fatalf("empty-body save rejected: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
