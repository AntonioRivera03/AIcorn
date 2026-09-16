package evals

import (
	"context"
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFixedSuiteAndGraderRejectWrongSolutions(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Cases) != 12 {
		t.Fatal("versioned suite changed; update documentation and recorded experiment")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python unavailable")
	}
	for _, c := range s.Cases {
		if c.Category != "coding" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			work := t.TempDir()
			os.WriteFile(filepath.Join(work, "solution.py"), []byte(c.Starter), 0600)
			if err := grade(context.Background(), python, work, nil, c, ""); err == nil {
				t.Fatal("broken starter passed")
			}
		})
	}
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "solution.py"), []byte("def solve(value):\n    return value * 2\n"), 0600)
	c := Case{Category: "coding", Checks: []Check{{Input: 3, Expected: 6}}}
	if err := grade(context.Background(), python, work, nil, c, ""); err != nil {
		t.Fatal(err)
	}
}
func TestTaskGraderUsesDatabasePostconditions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := appdb.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, p); err != nil {
		t.Fatal(err)
	}
	c := Case{ID: "attach-pr", Category: "task-manager", Prompt: "Attach a PR", Grader: "pr"}
	if err = seed(db, c); err != nil {
		t.Fatal(err)
	}
	if err = grade(context.Background(), "", "", db, c, "Done"); err == nil {
		t.Fatal("a claimed success without changes passed")
	}
	db.Exec("INSERT INTO task_github_link(task,url,kind,label) VALUES(101,'https://github.com/antoniorivera03/aicorn/pull/16','pull_request','Enhancements')")
	if err = grade(context.Background(), "", "", db, c, ""); err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE task SET name='Unexpected edit' WHERE id=101")
	if err = grade(context.Background(), "", "", db, c, ""); err == nil {
		t.Fatal("unrequested title edit passed")
	}
}
func TestReportMissingUsageAndDelta(t *testing.T) {
	if usageTokens(`{}`) != nil {
		t.Fatal("unknown usage became zero")
	}
	tokens := usageTokens(`{"total":{"inputTokens":10,"cachedInputTokens":2,"outputTokens":3,"totalTokens":13}}`)
	if tokens == nil || tokens.Total != 13 || tokens.CachedInput != 2 {
		t.Fatal(tokens)
	}
	r := &Report{Results: []Result{{Case: "a", Variant: "baseline", Usage: json.RawMessage(`{}`)}, {Case: "a", Variant: "verify-first", Passed: true, Tokens: tokens, Usage: json.RawMessage(`{}`)}}}
	prefix := filepath.Join(t.TempDir(), "report")
	if err := Save(prefix, r); err != nil {
		t.Fatal(err)
	}
	if r.DeltaPoints == nil || *r.DeltaPoints != 100 || r.Summary["baseline"].UsageReported != 0 {
		t.Fatalf("%+v", r)
	}
	raw, err := os.ReadFile(prefix + ".json")
	if err != nil || !json.Valid(raw) {
		t.Fatal(err)
	}
	if _, err = os.Stat(prefix + ".csv"); err != nil {
		t.Fatal(err)
	}
}
