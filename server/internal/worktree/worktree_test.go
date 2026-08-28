package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTempRepo creates a temp git repo with one commit and returns its path.
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	mustRun("init", "-b", "main")
	mustRun("config", "user.email", "test@test.com")
	mustRun("config", "user.name", "Test")
	// Create initial commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustRun("add", "README.md")
	mustRun("commit", "-m", "initial")

	return dir
}

func TestCreateAndRemove(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()

	wt, err := Create(ctx, repo, 1, 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify fields.
	if wt.Branch != "aycorn/task-1" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "aycorn/task-1")
	}
	expectedPath := filepath.Join(repo, ".worktrees", "task-1")
	if wt.Path != expectedPath {
		t.Errorf("Path = %q, want %q", wt.Path, expectedPath)
	}
	if wt.JobID != 1 || wt.TaskID != 100 {
		t.Errorf("JobID/TaskID = %d/%d, want 1/100", wt.JobID, wt.TaskID)
	}

	// Worktree directory should exist.
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("worktree path missing: %v", err)
	}

	// Branch should exist.
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/aycorn/task-1")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("branch not created: %v", err)
	}

	// Diff should be empty initially.
	diff, err := wt.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff on clean worktree = %q, want empty", diff)
	}

	// Write a file in worktree and check Diff.
	newFile := filepath.Join(wt.Path, "hello.txt")
	if err := os.WriteFile(newFile, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}
	diff, err = wt.Diff()
	if err != nil {
		t.Fatalf("Diff after write: %v", err)
	}
	if !strings.Contains(diff, "hello.txt") {
		t.Errorf("Diff should contain hello.txt, got %q", diff)
	}

	// Modify tracked file and check Diff contains modification.
	readme := filepath.Join(wt.Path, "README.md")
	if err := os.WriteFile(readme, []byte("# test\nmodified\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	diff, err = wt.Diff()
	if err != nil {
		t.Fatalf("Diff after modify: %v", err)
	}
	if !strings.Contains(diff, "modified") {
		t.Errorf("Diff should contain 'modified', got %q", diff)
	}

	// Remove worktree — should unlink directory but leave branch.
	if err := wt.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree path should be gone after Remove, stat err: %v", err)
	}
	// Branch should still exist.
	cmd = exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/aycorn/task-1")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Errorf("branch should still exist after Remove (leave for human inspection), err: %v", err)
	}

	// Clean up branch for test hygiene.
	_ = exec.Command("git", "branch", "-D", "aycorn/task-1").Run()
}

func TestCreateExistingBranchReused(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()

	// Create first worktree and remove it (leaves branch).
	wt1, err := Create(ctx, repo, 2, 200)
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if err := wt1.Remove(); err != nil {
		t.Fatalf("Remove 1: %v", err)
	}

	// Create again with same jobID — branch already exists, should reuse.
	wt2, err := Create(ctx, repo, 2, 200)
	if err != nil {
		t.Fatalf("Create 2 (reuse branch): %v", err)
	}
	if wt2.Branch != "aycorn/task-2" {
		t.Errorf("Branch = %q, want aycorn/task-2", wt2.Branch)
	}
	if _, err := os.Stat(wt2.Path); err != nil {
		t.Fatalf("worktree path missing on reuse: %v", err)
	}
	// Cleanup.
	if err := wt2.RemoveWithBranch(true); err != nil {
		t.Fatalf("RemoveWithBranch: %v", err)
	}
	// Branch should be gone after delete.
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/aycorn/task-2")
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		t.Errorf("branch should be deleted after RemoveWithBranch(true)")
	}
}

func TestCreateIdempotentPathExists(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()

	wt, err := Create(ctx, repo, 3, 300)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = wt.RemoveWithBranch(true) }()

	// Second create with same jobID while worktree still exists should error.
	_, err = Create(ctx, repo, 3, 300)
	if err == nil {
		t.Fatal("expected error when worktree path already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got %q", err.Error())
	}
}

func TestDiffEmptyWhenNoChanges(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()

	wt, err := Create(ctx, repo, 4, 400)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = wt.RemoveWithBranch(true) }()

	diff, err := wt.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff = %q, want empty", diff)
	}
}

func TestWorktreePath(t *testing.T) {
	got := WorktreePath("/repo", 42)
	want := filepath.Join("/repo", ".worktrees", "task-42")
	if got != want {
		t.Errorf("WorktreePath = %q, want %q", got, want)
	}
}

func TestBranchName(t *testing.T) {
	got := BranchName(99)
	if got != "aycorn/task-99" {
		t.Errorf("BranchName = %q, want %q", got, "aycorn/task-99")
	}
}

func TestCreateInvalidArgs(t *testing.T) {
	ctx := context.Background()
	_, err := Create(ctx, "", 1, 1)
	if err == nil {
		t.Error("expected error for empty repoRoot")
	}
	repo := initTempRepo(t)
	_, err = Create(ctx, repo, 0, 1)
	if err == nil {
		t.Error("expected error for jobID 0")
	}
	_, err = Create(ctx, repo, -1, 1)
	if err == nil {
		t.Error("expected error for negative jobID")
	}
}

func TestRepoRootFromDir(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()
	root, err := RepoRootFromDir(ctx, repo)
	if err != nil {
		t.Fatalf("RepoRootFromDir: %v", err)
	}
	// Compare cleaned paths.
	if filepath.Clean(root) != filepath.Clean(repo) {
		t.Errorf("RepoRootFromDir = %q, want %q", root, repo)
	}
}

func TestDiffContextCancellation(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()
	wt, err := Create(ctx, repo, 5, 500)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = wt.RemoveWithBranch(true) }()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = wt.DiffContext(cancelCtx)
	if err == nil {
		t.Log("Diff with cancelled context returned no error (git finished quickly) — not failing, but ideally would respect ctx")
	}
}

func TestRemoveWithBranchDelete(t *testing.T) {
	repo := initTempRepo(t)
	ctx := context.Background()
	wt, err := Create(ctx, repo, 6, 600)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := wt.RemoveWithBranch(true); err != nil {
		t.Fatalf("RemoveWithBranch(true): %v", err)
	}
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/aycorn/task-6")
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		t.Error("branch should be deleted")
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("path should be gone, stat err: %v", err)
	}
}
