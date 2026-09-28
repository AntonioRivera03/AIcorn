package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/repolink"
)

func TestRepositoryLinkHTTP(t *testing.T) {
	a, _ := requestAgentTestApp(t, "__valid_git__")
	db := a.projectRepo.DB
	a.repositoryService = &repolink.Service{DB: db}
	t.Cleanup(a.repositoryService.Wait)
	a.projectService = &services.ProjectService{ProjectRepo: a.projectRepo, TaskRepo: &repos.TaskRepo{DB: db}, ChecklistRepo: &repos.ChecklistRepo{DB: db}, WorkflowRepo: &repos.WorkflowRepo{DB: db}, StageRepo: &repos.StageRepo{DB: db}, Repositories: a.repositoryService}
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		rec := httptest.NewRecorder()
		a.routes().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	const link = "/api/project/1/settings/repository"
	var status repolink.Status
	if err := json.Unmarshal(call("GET", link, "", 200), &status); err != nil || status.Mode != repolink.Personal || !status.Linked || status.Path == "" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	for _, body := range []string{
		`{"mode":"official","url":"git@github.com:acme/app.git"}`,
		`{"url":"file:///srv/app.git"}`,
		`{"url":"https://github.com/acme/app?ref=main"}`,
		`{"mode":"svn"}`,
		`{"path":"relative/folder"}`,
		`{"branch":"main"}`,
	} {
		call("PUT", link, body, 400)
	}
	call("POST", link+"/fetch", "", 400) // only Official links are fetched
	call("GET", "/api/project/999/settings/repository", "", 404)

	// A whole-project PUT built from a stale copy leaves the link alone.
	call("PUT", "/api/project/1", `{"ID":1,"Name":"Renamed","Workflow":1,"DefaultView":"list","RepoMode":"","RepoPath":""}`, 200)
	var details struct {
		Project struct{ Name, RepoMode, RepoPath, RepoURL string }
	}
	if err := json.Unmarshal(call("GET", "/api/project/1", "", 200), &details); err != nil || details.Project.Name != "Renamed" || details.Project.RepoMode != "personal" || details.Project.RepoPath != status.Path {
		t.Fatalf("project = %+v, %v", details.Project, err)
	}

	t.Setenv("AYCORN_PREVIEW", "1")
	call("PUT", link, `{"path":"/tmp"}`, 403)
	call("POST", link+"/fetch", "", 403)
	call("GET", link, "", 200)
	t.Setenv("AYCORN_PREVIEW", "")

	// Deleting a project removes Aycorn's clone of its Official repository.
	if _, err := db.Exec(`UPDATE project SET repoMode='official', repoUrl='https://github.com/acme/app' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	clone, err := repolink.Locate(context.Background(), db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(clone.Root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(call("GET", link, "", 200), &status); err != nil || !status.Cloned || status.URL != "https://github.com/acme/app" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	call("DELETE", "/api/project/1", "", 200)
	a.repositoryService.Wait()
	if _, err = os.Stat(filepath.Dir(filepath.Dir(clone.Root))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clone kept after deleting its project: %v", err)
	}
}
