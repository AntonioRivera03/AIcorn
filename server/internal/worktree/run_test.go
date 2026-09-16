package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureIncludesCommitsAndUntrackedFilesWithoutChangingIndex(t *testing.T) {
	root := initTempRepo(t)
	ctx := context.Background()
	w, base, err := CreateRun(ctx, root, strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(w.Path, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "committed change\n")
	if err := runGit(ctx, w.Path, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(ctx, w.Path, "commit", "-m", "agent edit"); err != nil {
		t.Fatal(err)
	}
	write("new.txt", "untracked content\n")
	write("staged.txt", "staged content\n")
	if err := runGit(ctx, w.Path, "add", "staged.txt"); err != nil {
		t.Fatal(err)
	}
	before, err := gitOutput(ctx, w.Path, "diff", "--cached")
	if err != nil {
		t.Fatal(err)
	}
	patch, files, err := w.Capture(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	after, err := gitOutput(ctx, w.Path, "diff", "--cached")
	if err != nil {
		t.Fatal(err)
	}
	if before != after || len(files) != 3 {
		t.Fatalf("index changed or missing files: %v", files)
	}
	for _, content := range []string{"+committed change", "+untracked content", "+staged content"} {
		if !strings.Contains(patch, content) {
			t.Fatalf("missing %s in %s", content, patch)
		}
	}
	if _, _, err := CreateRun(ctx, root, strings.Repeat("a", 32)); err == nil {
		t.Fatal("reused existing workspace")
	}
	if raw, err := os.ReadFile(filepath.Join(w.Path, "new.txt")); err != nil || string(raw) != "untracked content\n" {
		t.Fatal("collision damaged existing workspace")
	}
}

func TestChatWorktreeResumeAndTurnSnapshot(t *testing.T) {
	root, w, _ := branchFixture(t)
	ctx := context.Background()
	branchFile(t, w.Path, "first.txt", "first\n")
	branchGit(t, w.Path, "add", "first.txt")
	before := branchGit(t, w.Path, "diff", "--cached")
	base, err := w.SnapshotTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	branchFile(t, w.Path, "second.txt", "second\n")
	patch, files, err := w.Capture(ctx, base)
	if err != nil || len(files) != 1 || files[0] != "second.txt" || strings.Contains(patch, "first.txt") {
		t.Fatalf("turn patch: %v %s %v", files, patch, err)
	}
	if branchGit(t, w.Path, "diff", "--cached") != before {
		t.Fatal("snapshot changed staging")
	}
	if resumed, err := ResumeRun(ctx, root, w.Path, w.Branch); err != nil || resumed.Path != w.Path {
		t.Fatalf("resume: %+v %v", resumed, err)
	}
	if _, err := ResumeRun(ctx, root, root, w.Branch); err == nil {
		t.Fatal("accepted mismatched path")
	}
	if _, err := ResumeRun(ctx, root, filepath.Join(root, "missing"), w.Branch); err == nil {
		t.Fatal("accepted missing checkout")
	}
}

func TestActiveChatWorktreeBlocksMergeAndPreviewSnapshot(t *testing.T) {
	root, w, base := branchFixture(t)
	ctx := context.Background()
	branchFile(t, w.Path, "change.txt", "chat change\n")
	preview := previewBranch(t, root, w, "main")
	release, err := ReserveRun(root, w.Branch)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if other, err := ReserveRun(root, w.Branch); !errors.Is(err, ErrMergeBlocked) {
		if other != nil {
			other()
		}
		t.Fatalf("second reservation: %v", err)
	}
	if _, err := MergeBranch(ctx, root, w.Branch, "main", preview.Token, 1, 1); !errors.Is(err, ErrMergeBlocked) {
		t.Fatalf("merged active chat: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "source")
	if _, err := SnapshotSource(ctx, root, w.Branch, base, true, dest); !errors.Is(err, ErrMergeBlocked) {
		t.Fatalf("snapshotted active chat: %v", err)
	}
	release()
	if _, err := SnapshotSource(ctx, root, w.Branch, base, true, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeBranch(ctx, root, w.Branch, "main", preview.Token, 1, 1); err != nil {
		t.Fatal(err)
	}
}
