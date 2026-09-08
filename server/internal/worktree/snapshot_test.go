package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func snapshotFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("committed"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	return root
}
func TestSnapshotIncludesUncommittedFilesWithoutChangingGit(t *testing.T) {
	root := snapshotFixture(t)
	ctx := context.Background()
	commit, _ := ResolveBranch(ctx, root, "main")
	for name, body := range map[string]string{"app.txt": "modified", "new.txt": "new", ".env": "secret", "app.db": "private"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := gitOutput(ctx, root, "status", "--porcelain=v1")
	index, _ := gitOutput(ctx, root, "write-tree")
	dest := filepath.Join(t.TempDir(), "source")
	snap, err := SnapshotSource(ctx, root, "main", commit, true, dest)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"app.txt": "modified", "new.txt": "new"} {
		data, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(data) != body {
			t.Fatalf("%s: %q %v", name, data, err)
		}
	}
	for _, name := range []string{".env", "app.db", ".git"} {
		if _, err := os.Stat(filepath.Join(dest, name)); !os.IsNotExist(err) {
			t.Fatalf("exported private file: %s", name)
		}
	}
	after, _ := gitOutput(ctx, root, "status", "--porcelain=v1")
	indexAfter, _ := gitOutput(ctx, root, "write-tree")
	head, _ := ResolveBranch(ctx, root, "main")
	if before != after || index != indexAfter || head != commit {
		t.Fatal("snapshot modified the working tree, index, or branch")
	}
	if len(snap.Digest) != 64 {
		t.Fatal("missing snapshot digest")
	}
	other, err := SnapshotSource(ctx, root, "main", commit, true, filepath.Join(t.TempDir(), "source"))
	if err != nil || other.Digest != snap.Digest {
		t.Fatal("snapshot is not deterministic")
	}
	clean := filepath.Join(t.TempDir(), "source")
	if _, err := SnapshotSource(ctx, root, "main", commit, false, clean); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(clean, "app.txt"))
	if string(data) != "committed" {
		t.Fatal("branch preview included dirty edits")
	}
}
func TestSnapshotRejectsLinksAndLFS(t *testing.T) {
	root := snapshotFixture(t)
	ctx := context.Background()
	commit, _ := ResolveBranch(ctx, root, "main")
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Skip(err)
	}
	if _, err := SnapshotSource(ctx, root, "main", commit, true, filepath.Join(t.TempDir(), "source")); err == nil || !strings.Contains(err.Error(), "links") {
		t.Fatalf("symlink accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset"), []byte("version https://git-lfs.github.com/spec/v1\noid sha256:123\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotSource(ctx, root, "main", commit, true, filepath.Join(t.TempDir(), "source")); err == nil || !strings.Contains(err.Error(), "LFS") {
		t.Fatalf("LFS pointer accepted: %v", err)
	}
}

func TestSnapshotPreservesSourceInBinDirectories(t *testing.T) {
	root := snapshotFixture(t)
	name := "server/assets/bin/bin.go"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("package bin\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := gitOutput(ctx, root, "add", name); err != nil {
		t.Fatal(err)
	}
	if _, err := gitOutput(ctx, root, "commit", "-m", "embedded build source"); err != nil {
		t.Fatal(err)
	}
	commit, err := ResolveBranch(ctx, root, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, dirty := range []bool{false, true} {
		dest := filepath.Join(t.TempDir(), "source")
		if _, err := SnapshotSource(ctx, root, "main", commit, dirty, dest); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(filepath.Join(dest, name)); err != nil || string(data) != "package bin\n" {
			t.Fatalf("source under bin directory was excluded (dirty=%v): %q %v", dirty, data, err)
		}
	}
}
