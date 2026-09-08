package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/environments"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

func TestEnvironmentHTTPAndPreviewGuard(t *testing.T) {
	a := aiTestApp(t, "")
	db := a.aiService.Jobs.DB
	a.projectRepo = &repos.ProjectRepo{DB: db}
	a.environmentService = &environments.Service{Store: &environments.Store{DB: db}, Runtime: &environments.Kubernetes{Token: "test"}}
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		if r.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	call("GET", "/api/health/ready", "", 200)
	call("GET", "/api/project/1/settings/environments", "", 200)
	call("PUT", "/api/project/1/settings/environments", `{"memoryMiB":512}`, 200)
	call("PUT", "/api/project/1/settings/environments", `{"port":80}`, 400)
	call("PUT", "/api/project/1/settings/environments", `{"autoPreview":true}`, 400)
	call("POST", "/api/environments/task/1", `{"jobId":999}`, 400)
	call("POST", "/api/environments/project/1", `{"branch":"main","privileged":true}`, 400)
	call("GET", "/api/environments/task/1", "", 200)
	call("GET", "/api/project/999/settings/environments", "", 404)
	call("DELETE", "/api/environment/999", "", 404)
	r := call("POST", "/api/environments/project/1/bulk", `{"ids":[999],"action":"stop"}`, 200)
	var result map[string]int
	if json.Unmarshal(r.Body.Bytes(), &result) != nil || result["skipped"] != 1 || result["success"] != 0 {
		t.Fatal("invalid bulk result")
	}
	request := httptest.NewRequest("POST", "http://127.0.0.1:8000/api/project", strings.NewReader(`{}`))
	request.Header.Set("Origin", "http://127.0.0.1:49200")
	response := httptest.NewRecorder()
	a.routes().ServeHTTP(response, request)
	if response.Code != 403 || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("preview origin can access main app")
	}
	t.Setenv("AYCORN_PREVIEW", "1")
	for _, path := range []string{"/api/ai/tasks/1/runs", "/api/task/1/request-agent", "/api/project/1/conductor/bulk", "/api/environments/project/1", "/api/environment/1/stop"} {
		call("POST", path, `{}`, 403)
	}
	call("GET", "/api/ai/tasks/1/branches", "", 403)
	call("GET", "/api/preview", "", 200)
	call("GET", "/api/health/ready", "", 200)
	call("GET", "/api/task/1", "", 200)
}

func TestPreviewSeedIsFreshAndPreservesEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if err = seedPreview(db); err != nil {
		t.Fatal(err)
	}
	projects, err := (&repos.ProjectRepo{DB: db}).All()
	if err != nil || len(projects) != 1 || projects[0].Name != "Preview sandbox" {
		t.Fatalf("preview project cannot be loaded by the application: %v %v", projects, err)
	}
	if _, err = db.Exec(`UPDATE project SET name='My preview edits'`); err != nil {
		t.Fatal(err)
	}
	if err = seedPreview(db); err != nil {
		t.Fatal(err)
	}
	var count int
	var name string
	if err = db.QueryRow(`SELECT COUNT(*),name FROM project`).Scan(&count, &name); err != nil || count != 1 || name != "My preview edits" {
		t.Fatalf("seed overwrote existing data: %d %s %v", count, name, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM task`).Scan(&count); err != nil || count != 1 {
		t.Fatal("sample task missing")
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM agent_job`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview seeded live automation")
	}
}
