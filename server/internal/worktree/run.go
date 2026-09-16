package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var runKeyPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// CreateRun uses a random run identity, so separate databases cannot collide.
// Existing paths are never pruned or reused automatically.
func CreateRun(ctx context.Context, root, key string) (*Worktree, string, error) {
	if !runKeyPattern.MatchString(key) {
		return nil, "", fmt.Errorf("invalid run identity")
	}
	base, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return nil, "", err
	}
	base = strings.TrimSpace(base)
	path := filepath.Join(root, ".worktrees", "run-"+key)
	branch := "aycorn/run-" + key
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("workspace already exists or is inaccessible: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, "", err
	}
	if err := runGit(ctx, root, "worktree", "add", "-b", branch, path, base); err != nil {
		return nil, "", err
	}
	return &Worktree{Path: path, RepoRoot: root, Branch: branch}, base, nil
}

// Capture uses a private index: include new files and agent commits without
// modifying the worktree's real staging area or creating a commit.
func (w *Worktree) Capture(ctx context.Context, base string) (string, []string, error) {
	run, cleanup, err := w.stagedIndex(ctx)
	if err != nil {
		return "", nil, err
	}
	defer cleanup()
	diff, err := run("diff", "--cached", "--no-ext-diff", "--no-color", base, "--")
	if err != nil {
		return "", nil, err
	}
	names, err := run("diff", "--cached", "--name-only", "-z", base, "--")
	if err != nil {
		return "", nil, err
	}
	files := []string{}
	for _, name := range strings.Split(names, "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	if len(diff) > 4*1024*1024 {
		return "", files, fmt.Errorf("patch exceeds 4 MiB; inspect the preserved workspace")
	}
	return diff, files, nil
}

// SnapshotTree records a turn's starting content without a commit or index edit.
func (w *Worktree) SnapshotTree(ctx context.Context) (string, error) {
	run, cleanup, err := w.stagedIndex(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()
	tree, err := run("write-tree")
	return strings.TrimSpace(tree), err
}
func (w *Worktree) stagedIndex(ctx context.Context) (func(...string) (string, error), func(), error) {
	temp, err := os.MkdirTemp("", "aycorn-patch-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(temp) }
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = w.Path
		env := []string{}
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "GIT_INDEX_FILE=") {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "GIT_INDEX_FILE="+filepath.Join(temp, "index"))
		raw, err := cmd.Output()
		return string(raw), err
	}
	if _, err = run("read-tree", "HEAD"); err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err = run("add", "-A", "--", "."); err != nil {
		cleanup()
		return nil, nil, err
	}
	return run, cleanup, nil
}

// ResumeRun verifies the preserved checkout is still the recorded branch in the
// recorded repository. A deleted or moved worktree requires a new conversation.
func ResumeRun(ctx context.Context, root, directory, branch string) (*Worktree, error) {
	actual, err := branchWorkspace(ctx, root, branch)
	if err != nil {
		return nil, err
	}
	expected, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil || actual != expected {
		return nil, fmt.Errorf("chat workspace changed or is missing; start a new conversation")
	}
	if err = workspaceReady(ctx, actual, false); err != nil {
		return nil, err
	}
	return &Worktree{Path: actual, RepoRoot: root, Branch: branch}, nil
}
