package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func branchGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	raw, err := gitOutput(context.Background(), path, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(raw)
}
func branchFile(t *testing.T, path, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func branchFixture(t *testing.T) (string, *Worktree, string) {
	t.Helper()
	root := initTempRepo(t)
	branchFile(t, root, ".gitignore", ".worktrees/\n")
	branchGit(t, root, "add", ".gitignore")
	branchGit(t, root, "commit", "-m", "ignore worktrees")
	w, base, err := CreateRun(context.Background(), root, strings.Repeat("b", 32))
	if err != nil {
		t.Fatal(err)
	}
	return root, w, base
}
func previewBranch(t *testing.T, root string, w *Worktree, target string) *MergePreview {
	t.Helper()
	p, err := PreviewMerge(context.Background(), root, w.Branch, target)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func mergeBranch(t *testing.T, root string, w *Worktree, p *MergePreview) *MergeResult {
	t.Helper()
	result, err := MergeBranch(context.Background(), root, w.Branch, p.Target, p.Token, 42, 7)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestMergeIncludesAllEditsAndIsIdempotent(t *testing.T) {
	root, w, base := branchFixture(t)
	branchFile(t, w.Path, "staged.txt", "staged\n")
	branchGit(t, w.Path, "add", "staged.txt")
	branchFile(t, w.Path, "staged.txt", "staged and unstaged\n")
	branchFile(t, w.Path, "new file\nname.txt", "new\n")
	if err := os.Remove(filepath.Join(w.Path, "README.md")); err != nil {
		t.Fatal(err)
	}
	before := branchGit(t, w.Path, "diff", "--cached")
	p := previewBranch(t, root, w, "")
	if p.Target != "main" || !p.Uncommitted || len(p.Files) != 3 || !strings.Contains(p.Diff, "+staged and unstaged") {
		t.Fatalf("%+v", p)
	}
	if branchGit(t, w.Path, "diff", "--cached") != before {
		t.Fatal("preview changed real index")
	}
	result := mergeBranch(t, root, w, p)
	if result.Target != "main" || result.Warning != "" {
		t.Fatalf("%+v", result)
	}
	if got := branchGit(t, root, "show", "main:staged.txt"); got != "staged and unstaged" {
		t.Fatal(got)
	}
	if branchGit(t, root, "symbolic-ref", "--short", "HEAD") != "main" {
		t.Fatal("switched checkout")
	}
	if branchGit(t, root, "status", "--porcelain") != "" || branchGit(t, w.Path, "status", "--porcelain") != "" {
		t.Fatal("dirty after merge")
	}
	state, err := InspectBranch(context.Background(), root, w.Branch, base)
	if err != nil || len(state.MergedInto) != 1 || state.MergedInto[0] != "main" {
		t.Fatalf("%+v %v", state, err)
	}
	p = previewBranch(t, root, w, "main")
	if !p.AlreadyMerged {
		t.Fatal("repeat merge not recognized")
	}
	if mergeBranch(t, root, w, p).Commit != result.Commit {
		t.Fatal("repeat merge created a commit")
	}
}
func TestMergeToUnopenedAndOtherWorktreeBranches(t *testing.T) {
	for _, checkedOut := range []bool{false, true} {
		t.Run(map[bool]string{false: "unopened", true: "other-worktree"}[checkedOut], func(t *testing.T) {
			root, w, _ := branchFixture(t)
			branchGit(t, root, "branch", "release")
			var destination string
			if checkedOut {
				destination = filepath.Join(t.TempDir(), "release with spaces")
				branchGit(t, root, "worktree", "add", destination, "release")
			}
			mainHead := branchGit(t, root, "rev-parse", "main")
			branchFile(t, w.Path, "agent.txt", "agent work\n")
			mergeBranch(t, root, w, previewBranch(t, root, w, "release"))
			if branchGit(t, root, "rev-parse", "main") != mainHead {
				t.Fatal("changed current branch")
			}
			if branchGit(t, root, "show", "release:agent.txt") != "agent work" {
				t.Fatal("missing merged file")
			}
			if checkedOut {
				raw, err := os.ReadFile(filepath.Join(destination, "agent.txt"))
				if err != nil || string(raw) != "agent work\n" {
					t.Fatal("destination worktree not updated", err)
				}
			}
		})
	}
}
func TestMergeDivergedBranchesPreservesHistory(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, root, "main.txt", "main\n")
	branchGit(t, root, "add", "main.txt")
	branchGit(t, root, "commit", "-m", "main work")
	branchFile(t, w.Path, "agent.txt", "agent\n")
	result := mergeBranch(t, root, w, previewBranch(t, root, w, "main"))
	if parents := strings.Fields(branchGit(t, root, "show", "-s", "--format=%P", result.Commit)); len(parents) != 2 {
		t.Fatalf("merge parents %v", parents)
	}
}
func TestMergeRejectsDirtyDestinationAndStaleReview(t *testing.T) {
	for _, change := range []string{"dirty", "source-file", "target-commit", "source-commit", "source-checkout"} {
		t.Run(change, func(t *testing.T) {
			root, w, base := branchFixture(t)
			branchFile(t, w.Path, "agent.txt", "agent\n")
			p := previewBranch(t, root, w, "main")
			switch change {
			case "dirty":
				branchFile(t, root, "README.md", "personal changes\n")
			case "source-file":
				branchFile(t, w.Path, "agent.txt", "changed since preview\n")
			case "target-commit":
				branchGit(t, root, "commit", "--allow-empty", "-m", "moved")
			case "source-commit":
				branchGit(t, w.Path, "commit", "--allow-empty", "-m", "moved")
			case "source-checkout":
				branchGit(t, w.Path, "switch", "-c", "different")
			}
			head := branchGit(t, root, "rev-parse", "main")
			_, err := MergeBranch(context.Background(), root, w.Branch, "main", p.Token, 42, 7)
			if !errors.Is(err, ErrMergeBlocked) {
				t.Fatalf("expected block: %v", err)
			}
			if branchGit(t, root, "rev-parse", "main") != head {
				t.Fatal("destination moved")
			}
			if change != "source-commit" && branchGit(t, root, "rev-parse", w.Branch) != base {
				t.Fatal("source committed before validation")
			}
		})
	}
}
func TestMergeConflictPreservesBothBranches(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, root, "README.md", "destination version\n")
	branchGit(t, root, "add", "README.md")
	branchGit(t, root, "commit", "-m", "destination edit")
	branchFile(t, w.Path, "README.md", "agent version\n")
	before := branchGit(t, root, "rev-parse", "HEAD")
	_, err := MergeBranch(context.Background(), root, w.Branch, "main", previewBranch(t, root, w, "main").Token, 42, 7)
	if !errors.Is(err, ErrMergeBlocked) || !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("conflict: %v", err)
	}
	if branchGit(t, root, "rev-parse", "HEAD") != before || branchGit(t, root, "status", "--porcelain") != "" {
		t.Fatal("conflict changed destination")
	}
	if branchGit(t, w.Path, "show", "HEAD:README.md") != "agent version" {
		t.Fatal("lost source")
	}
	if strings.Contains(branchGit(t, root, "worktree", "list", "--porcelain"), "aycorn-merge-") {
		t.Fatal("leaked temporary worktree")
	}
}
func TestMergeMissingWorkspaceAndInvalidBranches(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, w.Path, "agent.txt", "agent\n")
	branchGit(t, w.Path, "add", "agent.txt")
	branchGit(t, w.Path, "commit", "-m", "agent")
	branchGit(t, root, "worktree", "remove", w.Path)
	mergeBranch(t, root, w, previewBranch(t, root, w, "main"))
	for _, target := range []string{"--help", "main~1", "refs/heads/main", "missing", w.Branch} {
		if _, err := PreviewMerge(context.Background(), root, w.Branch, target); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	branchGit(t, root, "branch", "-d", w.Branch)
	if _, err := InspectBranch(context.Background(), root, w.Branch, ""); !errors.Is(err, ErrBranchUnavailable) {
		t.Fatal(err)
	}
}
func TestMergeDoesNotOverwriteIgnoredDestinationFile(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, root, ".git/info/exclude", "local.txt\n")
	branchFile(t, root, "local.txt", "personal ignored content\n")
	branchFile(t, w.Path, "local.txt", "agent\n")
	branchGit(t, w.Path, "add", "-f", "local.txt")
	branchGit(t, w.Path, "commit", "-m", "add file")
	before := branchGit(t, root, "rev-parse", "main")
	_, err := MergeBranch(context.Background(), root, w.Branch, "main", previewBranch(t, root, w, "main").Token, 42, 7)
	if err == nil {
		t.Fatal("overwrote ignored file")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "local.txt"))
	if string(raw) != "personal ignored content\n" || branchGit(t, root, "rev-parse", "main") != before {
		t.Fatal("destination damaged")
	}
}

func TestPreviewIncludesExplicitlyStagedIgnoredFiles(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, root, ".git/info/exclude", "ignored.txt\n")
	branchFile(t, w.Path, "ignored.txt", "explicitly staged\n")
	branchGit(t, w.Path, "add", "-f", "ignored.txt")
	p := previewBranch(t, root, w, "main")
	if len(p.Files) != 1 || p.Files[0] != "ignored.txt" {
		t.Fatalf("missing staged file: %+v", p)
	}
	mergeBranch(t, root, w, p)
	if branchGit(t, root, "show", "main:ignored.txt") != "explicitly staged" {
		t.Fatal("lost staged file")
	}
}

func TestMergeRejectsInProgressGitOperations(t *testing.T) {
	for _, source := range []bool{true, false} {
		t.Run(map[bool]string{true: "source", false: "destination"}[source], func(t *testing.T) {
			root, w, base := branchFixture(t)
			branchFile(t, w.Path, "agent.txt", "agent\n")
			p := previewBranch(t, root, w, "main")
			path := root
			if source {
				path = w.Path
			}
			marker := branchGit(t, path, "rev-parse", "--git-path", "CHERRY_PICK_HEAD")
			if !filepath.IsAbs(marker) {
				marker = filepath.Join(path, marker)
			}
			if err := os.WriteFile(marker, []byte(base+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := MergeBranch(context.Background(), root, w.Branch, "main", p.Token, 42, 7)
			if !errors.Is(err, ErrMergeBlocked) {
				t.Fatalf("expected block: %v", err)
			}
			if branchGit(t, root, "rev-parse", "main") != base {
				t.Fatal("destination moved")
			}
		})
	}
}

func TestMergeHonorsRepositoryMergeHook(t *testing.T) {
	root, w, _ := branchFixture(t)
	branchFile(t, root, "main.txt", "main work\n")
	branchGit(t, root, "add", "main.txt")
	branchGit(t, root, "commit", "-m", "main work")
	branchFile(t, w.Path, "agent.txt", "agent work\n")
	hook := filepath.Join(root, ".git", "hooks", "pre-merge-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'merge validation failed' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	before := branchGit(t, root, "rev-parse", "main")
	_, err := MergeBranch(context.Background(), root, w.Branch, "main", previewBranch(t, root, w, "main").Token, 42, 7)
	if !errors.Is(err, ErrMergeBlocked) || !strings.Contains(err.Error(), "merge validation failed") {
		t.Fatalf("hook bypassed: %v", err)
	}
	if branchGit(t, root, "rev-parse", "main") != before {
		t.Fatal("destination changed")
	}
}
