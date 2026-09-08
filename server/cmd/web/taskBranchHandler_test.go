package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worker"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

func TestTaskBranchRoutesUseStoredAssociationAndMerge(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.test")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".worktrees/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".gitignore")
	git("commit", "-m", "base")
	app := aiTestApp(t, root)
	job, err := app.aiService.Start(context.Background(), 1, services.AIRunInput{Intent: "implement"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.New(app.agentJobService, failingEditHarness{}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, want int) string {
		t.Helper()
		rec := httptest.NewRecorder()
		app.routes().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec.Body.String()
	}
	// Moving the project's configured repo must not redirect a historical run's merge.
	if _, err := app.aiService.Jobs.DB.Exec("UPDATE project SET repoPath=? WHERE id=1", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var branches []models.TaskBranch
	if err := json.Unmarshal([]byte(request("GET", "/api/ai/tasks/1/branches", "", 200)), &branches); err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 || branches[0].RepoPath != root || !branches[0].Dirty || branches[0].JobID != job.ID {
		t.Fatalf("%+v", branches)
	}
	url := "/api/ai/tasks/1/branches/" + fmtInt(job.ID) + "/merge"
	request("GET", "/api/ai/tasks/999/branches/"+fmtInt(job.ID)+"/merge", "", 404)
	request("GET", "/api/ai/tasks/0/branches", "", 400)
	request("POST", url, `{"target":"main"}`, 400)
	request("GET", url+"?target=--help", "", 400)
	if _, err := app.aiService.Jobs.DB.Exec("UPDATE agent_job SET status='running' WHERE id=?", job.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", url, "", 409)
	if _, err := app.aiService.Jobs.DB.Exec("UPDATE agent_job SET status='failed' WHERE id=?", job.ID); err != nil {
		t.Fatal(err)
	}
	var preview worktree.MergePreview
	if err := json.Unmarshal([]byte(request("GET", url, "", 200)), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Target != "main" || !preview.Uncommitted || len(preview.Files) != 1 {
		t.Fatalf("%+v", preview)
	}
	request("POST", url, `{"target":"main","token":"stale"}`, 409)
	// An agent-owned destination must not be changed while its run is active.
	db := app.aiService.Jobs.DB
	if _, err := db.Exec("INSERT INTO task(id,checklist,stage,type,name,priority) SELECT 2,checklist,stage,type,'other','Medium' FROM task WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec("INSERT INTO agent_job(task,status,requestJson) VALUES(2,'running',?)", `{"repoPath":"`+root+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	destinationJob, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO agent_run(job,artifactJson) VALUES(?,?)", destinationJob, `{"branch":"main"}`); err != nil {
		t.Fatal(err)
	}
	var blocked worktree.MergePreview
	if err := json.Unmarshal([]byte(request("GET", url, "", 200)), &blocked); err != nil || blocked.Blocked == "" {
		t.Fatalf("active target not blocked: %+v %v", blocked, err)
	}
	payload, _ := json.Marshal(map[string]string{"target": preview.Target, "token": preview.Token})
	request("POST", url, string(payload), 409)
	if _, err := db.Exec("UPDATE agent_job SET status='completed' WHERE id=?", destinationJob); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"target": preview.Target, "token": preview.Token})
	request("POST", url, string(raw), 200)
	if err := json.Unmarshal([]byte(request("GET", "/api/ai/tasks/1/branches", "", 200)), &branches); err != nil {
		t.Fatal(err)
	}
	if branches[0].Dirty || len(branches[0].MergedInto) != 1 || branches[0].MergedInto[0] != "main" {
		t.Fatalf("%+v", branches)
	}
	if content, err := os.ReadFile(filepath.Join(root, "new.txt")); err != nil || !strings.Contains(string(content), "preserve this partial change") {
		t.Fatal("merge missing agent file", err)
	}
}
