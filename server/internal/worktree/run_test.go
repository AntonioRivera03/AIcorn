package worktree

import (
	"context"
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
