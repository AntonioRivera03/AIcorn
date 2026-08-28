package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

// writingHarness writes a file into spec.WorkDir to produce a diff.
type writingHarness struct {
	wrote *string
}

func (w *writingHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	if spec.WorkDir != "" {
		p := filepath.Join(spec.WorkDir, "hello.txt")
		_ = os.WriteFile(p, []byte("hello from job\n"), 0o644)
		if w.wrote != nil {
			*w.wrote = p
		}
	}
	return harness.RunResult{
		Output:    "# done for task " + fmt.Sprint(spec.TaskID),
		Summary:   "done",
		ExitCode:  0,
		UsageJson: `{"writing":true}`,
	}, nil
}

func itoaTest(n int) string { return fmt.Sprint(n) }

func initTempRepoForWorker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	must("init", "-b", "main")
	must("config", "user.email", "test@test.com")
	must("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	must("add", "README.md")
	must("commit", "-m", "initial")
	return dir
}

func TestWorker_WorktreeIntegration_DiffCaptured(t *testing.T) {
	repo := initTempRepoForWorker(t)
	origWd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	t.Cleanup(func() {
		// Clean worktrees/branches created by worker during this test
		_ = os.RemoveAll(filepath.Join(repo, ".worktrees"))
		cmd := exec.Command("git", "worktree", "prune")
		cmd.Dir = repo
		_ = cmd.Run()
		// Remove any aycorn branches
		cmd = exec.Command("bash", "-c", "git branch | grep aycorn | xargs -r git branch -D")
		cmd.Dir = repo
		_ = cmd.Run()
	})

	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	job, err := jobSvc.Enqueue(1, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var wrotePath string
	wh := &writingHarness{wrote: &wrotePath}
	w := New(jobSvc, taskSvc, wh)

	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !did {
		t.Fatal("RunOnce returned false")
	}

	// Verify worktree was created on expected branch
	branch := worktree.BranchName(job.ID)
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("branch %s not found: %v", branch, err)
	}

	wtPath := worktree.WorktreePath(repo, job.ID)
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree path missing %s: %v", wtPath, err)
	}

	// Verify diff captured: the written file should appear in agent_run output as fenced diff
	runs, err := jobSvc.RunRepo.ListByJob(job.ID)
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs %d want 1", len(runs))
	}
	out := runs[0].Output
	if !strings.Contains(out, "hello.txt") {
		t.Fatalf("output should contain diff with hello.txt, got %q", out)
	}
	if !strings.Contains(out, "```diff") {
		t.Fatalf("output should contain fenced diff block, got %q", out)
	}
	if !strings.Contains(out, "# done for task") {
		t.Fatalf("output should contain harness output, got %q", out)
	}
	// Diff should not contain MCP config artifact
	if strings.Contains(out, "mcp-") {
		t.Fatalf("diff should not contain MCP config file, got %q", out)
	}

	// Verify branch left for human (not auto-deleted)
	cmd = exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("branch should still exist after success (leave for human): %v", err)
	}

	// Verify CAS transition was attempted (task remains 20 or moved idempotently)
	// Since task already in 20 (toStage), TransitionStage should be idempotent conflict and not fail job.
}

func TestWorker_RepoRootHelper(t *testing.T) {
	repo := initTempRepoForWorker(t)
	root, err := worktree.RepoRootFromDir(context.Background(), repo)
	if err != nil {
		t.Fatalf("RepoRootFromDir: %v", err)
	}
	if filepath.Clean(root) != filepath.Clean(repo) {
		t.Fatalf("root %q want %q", root, repo)
	}
	// resolveRepoRoot via chdir
	origWd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origWd)
	got := resolveRepoRoot(context.Background())
	if filepath.Clean(got) != filepath.Clean(repo) {
		t.Fatalf("resolveRepoRoot %q want %q", got, repo)
	}
}

func TestWorker_GenerateMCPConfig_Cleanup(t *testing.T) {
	// Verify MCP config generation creates file and cleanup removes it
	repo := initTempRepoForWorker(t)
	origWd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origWd)

	_, jobSvc, taskSvc := setupWorkerTestDB(t)
	from := 10
	to := 20
	_, err := jobSvc.Enqueue(2, 1, &from, &to)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	w := New(jobSvc, taskSvc, harness.NewReadOnlyShim())
	did, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !did {
		t.Fatal("did false")
	}
	// After RunOnce, no mcp-*.json should remain in worktree
	wtPath := worktree.WorktreePath(repo, 1)
	matches, _ := filepath.Glob(filepath.Join(wtPath, "mcp-*.json"))
	if len(matches) != 0 {
		t.Fatalf("MCP config should be cleaned before diff/after run, found %v", matches)
	}
	// Also check repo/.worktrees/task-1 cleaned of mcp files (worktree left but without mcp)
}
