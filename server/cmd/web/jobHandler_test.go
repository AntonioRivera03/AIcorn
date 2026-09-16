package main

import (
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/jobs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTemplatesAndJobsHTTP(t *testing.T) {
	a := conductorTestApp(t)
	if _, err := a.aiService.Jobs.DB.Exec("INSERT OR IGNORE INTO project_task_type(project,task_type) VALUES(1,1)"); err != nil {
		t.Fatal(err)
	}
	a.jobService = &jobs.Service{DB: a.aiService.Jobs.DB, AI: a.aiService, Conductor: a.conductorService}
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		if r.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	call("PUT", "/api/project/1/settings/conductor", `{"planningStage":2,"workingStage":3,"completionStage":4,"useRepository":false,"conductorAgentId":1}`, 200)
	base := "/api/project/1/automation/templates"
	r := call("POST", base, "", 201)
	var tpl jobs.Template
	if err := json.Unmarshal(r.Body.Bytes(), &tpl); err != nil {
		t.Fatal(err)
	}
	item := base + "/" + fmtInt(tpl.ID)
	tpl.Name = "Audit"
	tpl.Body = "Check dependencies"
	tpl.Title = "New audit"
	raw, _ := json.Marshal(tpl)
	call("PUT", item, string(raw), 200)
	call("PUT", item, string(raw), 409)
	call("PUT", item, `{"unknown":true}`, 400)
	call("POST", item+"/instantiate", "", 201)
	call("GET", "/api/project/999/automation/templates/"+fmtInt(tpl.ID), "", 404)
	r = call("POST", item+"/job", "", 201)
	var j jobs.Job
	if err := json.Unmarshal(r.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	url := "/api/project/1/automation/jobs/" + fmtInt(j.ID)
	j.AgentID = 1
	j.Schedule = "0 9 * * *"
	j.Timezone = "America/Chicago"
	raw, _ = json.Marshal(j)
	call("PUT", url, string(raw), 200)
	call("GET", "/api/project/1/automation/jobs", "", 200)
	call("POST", url+"/run", `{"key":"click"}`, 202)
	call("POST", url+"/run", `{"key":"click"}`, 202)
	call("POST", url+"/run", `{"key":"another"}`, 409)
	call("POST", url+"/run", `{"key":"a"} {}`, 400)
	call("POST", url+"/run", `{}`, 400)
	call("GET", url+"/runs", "", 200)
	call("DELETE", url, "", 409)
	call("DELETE", item, "", 400)
}

func TestPreviewBlocksJobExecution(t *testing.T) {
	t.Setenv("AYCORN_PREVIEW", "1")
	a := conductorTestApp(t)
	a.jobService = &jobs.Service{DB: a.aiService.Jobs.DB, AI: a.aiService, Conductor: a.conductorService}
	for _, path := range []string{"/api/project/1/automation/jobs/1/run", "/api/project/1/automation/jobs/1", "/api/project/1/automation/templates/1/job"} {
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
		if r.Code != 403 {
			t.Fatalf("preview accepted %s: %d", path, r.Code)
		}
	}
}
