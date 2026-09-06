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
	temp, err := os.MkdirTemp("", "aycorn-patch-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(temp)
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
		return "", nil, err
	}
	if _, err = run("add", "-A", "--", "."); err != nil {
		return "", nil, err
	}
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
