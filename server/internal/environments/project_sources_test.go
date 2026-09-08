package environments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sourceGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeProjectSource(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectWorkingTreePreviewIncludesUncommittedPreviewSupport(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "committed"})
	configureFixture(t, store, root)
	if _, err := store.UpdateSettings(1, map[string]json.RawMessage{"profile": json.RawMessage(`"aycorn"`)}); err != nil {
		t.Fatal(err)
	}
	// This is the original regression: preview support exists on disk, but is
	// not in the selected branch's commit yet. Preserve staged edits as well.
	writeProjectSource(t, root, "app.txt", "staged")
	sourceGit(t, root, "add", "app.txt")
	writeProjectSource(t, root, "app.txt", "working tree")
	marker := "package main\n\nconst PreviewProtocolVersion = 1\n"
	writeProjectSource(t, root, "server/cmd/web/preview.go", marker)
	writeProjectSource(t, root, "new.txt", "untracked source")
	beforeHead := sourceGit(t, root, "rev-parse", "HEAD")
	beforeStatus := sourceGit(t, root, "--no-optional-locks", "status", "--porcelain=v1")
	beforeIndex, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: store, Root: t.TempDir()}
	ctx := context.Background()
	committed, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if committed.IncludeChanges {
		t.Fatal("project preview silently opted into working tree changes")
	}
	if err = s.snapshot(ctx, committed, func(string) {}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "working tree") {
		t.Fatalf("missing preview support should explain the working tree option: %v", err)
	}
	working, err := s.Create(ctx, 1, CreateInput{Branch: "main", IncludeChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if !working.IncludeChanges || working.JobID != 0 {
		t.Fatalf("project source choice was not retained: %#v", working)
	}
	if err = s.snapshot(ctx, working, func(string) {}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"app.txt": "working tree", "server/cmd/web/preview.go": marker, "new.txt": "untracked source"} {
		got, err := os.ReadFile(filepath.Join(s.directory(working.ID), "source", name))
		if err != nil || string(got) != want {
			t.Fatalf("snapshot %s = %q, %v; want %q", name, got, err, want)
		}
	}
	if len(working.Digest) != 64 || working.Commit != beforeHead {
		t.Fatalf("working tree snapshot lost its source identity: %#v", working)
	}
	afterIndex, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeIndex, afterIndex) || sourceGit(t, root, "rev-parse", "HEAD") != beforeHead || sourceGit(t, root, "--no-optional-locks", "status", "--porcelain=v1") != beforeStatus {
		t.Fatal("preview capture changed the source branch, index, or working tree")
	}
}

func TestProjectCommittedPreviewRetainsRequestedCommit(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "original commit"})
	configureFixture(t, store, root)
	s := &Service{Store: store, Root: t.TempDir()}
	ctx := context.Background()
	e, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	writeProjectSource(t, root, "app.txt", "new commit")
	sourceGit(t, root, "add", "app.txt")
	sourceGit(t, root, "commit", "-m", "advance branch after requesting preview")
	writeProjectSource(t, root, "app.txt", "uncommitted changes")
	if err = s.snapshot(ctx, e, func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.directory(e.ID), "source", "app.txt"))
	if err != nil || string(got) != "original commit" {
		t.Fatalf("committed preview followed newer branch or working tree content: %q, %v", got, err)
	}
}

func TestProjectPreviewRebuildPreservesSourceMode(t *testing.T) {
	for _, includeChanges := range []bool{false, true} {
		name := "committed"
		if includeChanges {
			name = "working tree"
		}
		t.Run(name, func(t *testing.T) {
			store := testStore(t)
			root := repoFixture(t, map[string]string{"app.txt": "original"})
			configureFixture(t, store, root)
			s := &Service{Store: store}
			ctx := context.Background()
			e, err := s.Create(ctx, 1, CreateInput{Branch: "main", IncludeChanges: includeChanges})
			if err != nil {
				t.Fatal(err)
			}
			writeProjectSource(t, root, "app.txt", "updated")
			sourceGit(t, root, "add", "app.txt")
			sourceGit(t, root, "commit", "-m", "update source")
			rebuilt, err := s.Rebuild(ctx, e.ID, CreateInput{IncludeChanges: !includeChanges})
			if err != nil {
				t.Fatal(err)
			}
			if rebuilt.ID == e.ID || rebuilt.IncludeChanges != includeChanges || rebuilt.Branch != "main" || rebuilt.JobID != 0 || rebuilt.Commit == e.Commit {
				t.Fatalf("rebuild did not preserve mode while capturing a fresh revision: %#v", rebuilt)
			}
			original, err := store.Get(e.ID)
			if err != nil || original.Commit != e.Commit || original.IncludeChanges != includeChanges {
				t.Fatalf("rebuild changed the original preview: %#v, %v", original, err)
			}
		})
	}
}

func insertSourceAgentRun(t *testing.T, store *Store, repo, branch, status string) {
	t.Helper()
	var task int
	if err := store.DB.QueryRow(`INSERT INTO task(checklist,stage,type,name,priority) SELECT checklist,stage,type,'Agent task',priority FROM task WHERE id=1 RETURNING id`).Scan(&task); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]string{"repoPath": repo})
	artifact, _ := json.Marshal(map[string]string{"branch": branch})
	result, err := store.DB.Exec(`INSERT INTO agent_job(task,status,requestJson) VALUES(?,?,?)`, task, status, string(request))
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`INSERT INTO agent_run(job,artifactJson) VALUES(?,?)`, id, string(artifact)); err != nil {
		t.Fatal(err)
	}
}

func TestProjectWorkingTreePreviewRejectsActiveBranchAtCreateAndCapture(t *testing.T) {
	for _, status := range []string{"pending", "claimed", "running", "canceling"} {
		t.Run(status, func(t *testing.T) {
			store := testStore(t)
			root := repoFixture(t, map[string]string{"app.txt": "original"})
			configureFixture(t, store, root)
			s := &Service{Store: store, Root: t.TempDir()}
			ctx := context.Background()
			e, err := s.Create(ctx, 1, CreateInput{Branch: "main", IncludeChanges: true})
			if err != nil {
				t.Fatal(err)
			}
			insertSourceAgentRun(t, store, root, "main", status)
			if _, err = s.Create(ctx, 1, CreateInput{Branch: "main", IncludeChanges: true}); !errors.Is(err, ErrConflict) {
				t.Fatalf("create accepted a branch with an active %s run: %v", status, err)
			}
			if err = s.snapshot(ctx, e, func(string) {}); err == nil {
				t.Fatalf("capture accepted an agent that became %s after preview creation", status)
			}
			if e.Digest != "" {
				t.Fatal("rejected capture recorded a source digest")
			}
			if _, err = os.Stat(filepath.Join(s.directory(e.ID), "source")); !os.IsNotExist(err) {
				t.Fatalf("rejected capture published source: %v", err)
			}
			committed, err := s.Create(ctx, 1, CreateInput{Branch: "main"})
			if err != nil {
				t.Fatalf("active run should not block immutable committed previews: %v", err)
			}
			if err = s.snapshot(ctx, committed, func(string) {}); err != nil {
				t.Fatalf("active run should not block committed source capture: %v", err)
			}
		})
	}
}

func TestProjectWorkingTreePreviewIgnoresUnrelatedAndCompletedRuns(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "original"})
	configureFixture(t, store, root)
	insertSourceAgentRun(t, store, root, "another-branch", "running")
	insertSourceAgentRun(t, store, t.TempDir(), "main", "running")
	insertSourceAgentRun(t, store, root, "main", "completed")
	s := &Service{Store: store, Root: t.TempDir()}
	ctx := context.Background()
	e, err := s.Create(ctx, 1, CreateInput{Branch: "main", IncludeChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.snapshot(ctx, e, func(string) {}); err != nil {
		t.Fatalf("unrelated or completed run prevented source capture: %v", err)
	}
}

func TestProjectWorkingTreePreviewRequiresCheckedOutBranch(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "original"})
	configureFixture(t, store, root)
	sourceGit(t, root, "branch", "not-checked-out")
	s := &Service{Store: store, Root: t.TempDir()}
	ctx := context.Background()
	if _, err := s.Create(ctx, 1, CreateInput{Branch: "not-checked-out", IncludeChanges: true}); err == nil {
		t.Fatal("accepted working tree source for a branch with no worktree")
	}
	e, err := s.Create(ctx, 1, CreateInput{Branch: "not-checked-out"})
	if err != nil {
		t.Fatalf("branch without a worktree should still support committed previews: %v", err)
	}
	if err = s.snapshot(ctx, e, func(string) {}); err != nil {
		t.Fatal(err)
	}
}

func TestProjectBranchSourcesAndCaptureSelectTheBranchWorktree(t *testing.T) {
	store := testStore(t)
	root := repoFixture(t, map[string]string{"app.txt": "committed"})
	configureFixture(t, store, root)
	other := filepath.Join(t.TempDir(), "feature workspace")
	sourceGit(t, root, "worktree", "add", "-b", "feature", other)
	sourceGit(t, root, "branch", "no-worktree")
	writeProjectSource(t, root, "app.txt", "main edits")
	writeProjectSource(t, other, "app.txt", "feature edits")
	s := &Service{Store: store, Root: t.TempDir()}
	ctx := context.Background()
	sources, err := s.BranchSources(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 || sources[0].Name != "feature" || sources[0].Current || !sources[0].HasWorktree || sources[1].Name != "main" || !sources[1].Current || !sources[1].HasWorktree || sources[2].Name != "no-worktree" || sources[2].Current || sources[2].HasWorktree {
		t.Fatalf("project branch metadata does not describe available sources: %#v", sources)
	}
	branches, err := s.Branches(ctx, 1)
	if err != nil || !reflect.DeepEqual(branches, []string{"feature", "main", "no-worktree"}) {
		t.Fatalf("plain branch list changed: %#v, %v", branches, err)
	}
	e, err := s.Create(ctx, 1, CreateInput{Branch: "feature", IncludeChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.snapshot(ctx, e, func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.directory(e.ID), "source", "app.txt"))
	if err != nil || string(got) != "feature edits" {
		t.Fatalf("selected branch preview read the wrong worktree: %q, %v", got, err)
	}
}
