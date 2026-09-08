package worktree

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const snapshotLimit = 128 << 20
const snapshotFileLimit = 16 << 20

type SourceSnapshot struct {
	Commit   string   `json:"commit"`
	Digest   string   `json:"digest"`
	Excluded []string `json:"excluded"`
}

type BranchSource struct {
	Name        string `json:"name"`
	Current     bool   `json:"current"`
	HasWorktree bool   `json:"hasWorktree"`
}

// Branch source choices describe Git's actual checkouts, not the server's cwd.
func LocalBranchSources(ctx context.Context, root string) ([]BranchSource, error) {
	branches, err := LocalBranches(ctx, root)
	if err != nil {
		return nil, err
	}
	current, err := gitOutput(ctx, root, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	raw, err := gitOutput(ctx, root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	checkedOut := map[string]bool{}
	var directory string
	for _, field := range strings.Split(raw, "\x00") {
		if strings.HasPrefix(field, "worktree ") {
			directory = strings.TrimPrefix(field, "worktree ")
		}
		if strings.HasPrefix(field, "branch refs/heads/") {
			info, err := os.Stat(directory)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			checkedOut[strings.TrimPrefix(field, "branch refs/heads/")] = err == nil && info.IsDir()
		}
	}
	result := make([]BranchSource, 0, len(branches))
	for _, branch := range branches {
		result = append(result, BranchSource{Name: branch, Current: branch == strings.TrimSpace(current), HasWorktree: checkedOut[branch]})
	}
	return result, nil
}

func LocalBranches(ctx context.Context, root string) ([]string, error) {
	out, err := gitOutput(ctx, root, "for-each-ref", "--format=%(refname:strip=2)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for _, b := range strings.Split(strings.TrimSpace(out), "\n") {
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}
func ResolveBranch(ctx context.Context, root, branch string) (string, error) {
	return localRef(ctx, root, branch)
}

func CurrentWorkspaceDigest(ctx context.Context, root, branch string) (string, error) {
	mergeMu.Lock()
	defer mergeMu.Unlock()
	workspace, err := branchWorkspace(ctx, root, branch)
	if err != nil {
		return "", err
	}
	if workspace == "" {
		return "", fmt.Errorf("source worktree no longer exists")
	}
	digest, _, err := snapshotWorkspace(ctx, workspace, "")
	return digest, err
}

func excludedSource(name string) bool {
	for _, part := range strings.Split(name, "/") {
		lower := strings.ToLower(part)
		switch lower {
		case ".git", ".worktrees", "node_modules", ".env", ".ssh", ".aws", ".azure", ".kube", ".codex", ".npmrc", ".netrc", ".docker", ".dockerignore", "backups", ".codegraph", ".cache", "dist":
			return true
		}
		if strings.HasPrefix(lower, ".env.") || strings.HasSuffix(lower, ".db") || strings.Contains(lower, ".db-") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
			return true
		}
	}
	return false
}
func safeSnapshotName(name string) bool {
	return name != "" && path.Clean(name) == name && !path.IsAbs(name) && name != ".." && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\\x00")
}

// SnapshotSource never stages, commits, follows links, or invokes checkout filters.
// Committed previews use Git blobs at the recorded commit. Working-tree previews
// read the inactive worktree twice and reject concurrent edits. App merges share this lock.
func SnapshotSource(ctx context.Context, root, branch, commit string, includeChanges bool, destination string) (*SourceSnapshot, error) {
	mergeMu.Lock()
	defer mergeMu.Unlock()
	current, err := localRef(ctx, root, branch)
	if err != nil {
		return nil, err
	}
	if commit == "" {
		commit = current
	}
	if includeChanges && current != commit {
		return nil, fmt.Errorf("source branch moved before snapshot; create a new preview")
	}
	entries, err := gitOutput(ctx, root, "ls-tree", "-r", commit)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(entries, "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return nil, fmt.Errorf("submodules need an explicit export recipe; snapshot was not created")
		}
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return nil, err
	}
	result := &SourceSnapshot{Commit: commit, Excluded: []string{}}
	if includeChanges {
		workspace, err := branchWorkspace(ctx, root, branch)
		if err != nil {
			return nil, err
		}
		if workspace == "" {
			return nil, fmt.Errorf("this branch has no working tree; choose Latest commit or check out the branch first")
		}
		if err = workspaceReady(ctx, workspace, false); err != nil {
			return nil, err
		}
		result.Digest, result.Excluded, err = snapshotWorkspace(ctx, workspace, destination)
		if err != nil {
			return nil, err
		}
		second, _, err := snapshotWorkspace(ctx, workspace, "")
		if err != nil {
			return nil, err
		}
		current, err = localRef(ctx, root, branch)
		if err != nil {
			return nil, err
		}
		if second != result.Digest || current != commit {
			return nil, fmt.Errorf("source files changed during snapshot; retry after editing finishes")
		}
		currentWorkspace, err := branchWorkspace(ctx, root, branch)
		if err != nil || currentWorkspace != workspace {
			return nil, fmt.Errorf("the source branch checkout changed during snapshot; create a new preview")
		}
		return result, nil
	}
	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar", commit)
	cmd.Dir = root
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	archive := tar.NewReader(reader)
	var total int64
	hash := sha256.New()
	count := 0
	for {
		header, readErr := archive.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := header.Name
		if !safeSnapshotName(name) {
			err = fmt.Errorf("invalid source path %q", name)
			break
		}
		if excludedSource(name) {
			result.Excluded = append(result.Excluded, name)
			continue
		}
		if header.Typeflag != tar.TypeReg {
			err = fmt.Errorf("source links are not supported: %s", name)
			break
		}
		if header.Size > snapshotFileLimit || total+header.Size > snapshotLimit || count >= 10000 {
			err = fmt.Errorf("source exceeds snapshot limits (128 MiB total, 16 MiB per file, 10000 files)")
			break
		}
		data, readErr := io.ReadAll(io.LimitReader(archive, snapshotFileLimit+1))
		if readErr != nil {
			err = readErr
			break
		}
		if err = writeSnapshotFile(destination, name, data, header.Mode, hash); err != nil {
			break
		}
		total += int64(len(data))
		count++
	}
	if err != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if err != nil {
		return nil, err
	}
	if waitErr != nil {
		return nil, waitErr
	}
	result.Digest = hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func snapshotWorkspace(ctx context.Context, workspace, destination string) (string, []string, error) {
	raw, err := gitOutput(ctx, workspace, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", nil, err
	}
	names := strings.Split(raw, "\x00")
	sort.Strings(names)
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	excluded := []string{}
	hash := sha256.New()
	var total int64
	seen := map[string]bool{}
	for _, name := range names {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if !safeSnapshotName(name) {
			return "", nil, fmt.Errorf("invalid source path %q", name)
		}
		if excludedSource(name) {
			excluded = append(excluded, name)
			continue
		}
		// Reject symlink parents as well as leaves; OpenRoot additionally prevents
		// escapes if an external editor replaces a directory during this read.
		for parent := name; parent != "."; parent = path.Dir(parent) {
			info, e := root.Lstat(parent)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return "", nil, e
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", nil, fmt.Errorf("source links are not supported: %s", parent)
			}
		}
		info, err := root.Stat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if !info.Mode().IsRegular() {
			return "", nil, fmt.Errorf("unsupported source file: %s", name)
		}
		if info.Size() > snapshotFileLimit || total+info.Size() > snapshotLimit || len(seen) > 10000 {
			return "", nil, fmt.Errorf("source exceeds snapshot limits (128 MiB total, 16 MiB per file, 10000 files)")
		}
		f, err := root.Open(name)
		if err != nil {
			return "", nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, snapshotFileLimit+1))
		_ = f.Close()
		if err != nil {
			return "", nil, err
		}
		if len(data) > snapshotFileLimit {
			return "", nil, fmt.Errorf("source file grew past size limit")
		}
		if err = writeSnapshotFile(destination, name, data, int64(info.Mode().Perm()), hash); err != nil {
			return "", nil, err
		}
		total += int64(len(data))
	}
	return hex.EncodeToString(hash.Sum(nil)), excluded, nil
}

func writeSnapshotFile(destination, name string, data []byte, mode int64, hash io.Writer) error {
	if strings.HasPrefix(string(data[:min(len(data), 128)]), "version https://git-lfs.github.com/spec/v1") {
		return fmt.Errorf("Git LFS pointer %s requires materialized source before previewing", name)
	}
	permissions := os.FileMode(0644)
	if mode&0111 != 0 {
		permissions = 0755
	}
	fmt.Fprintf(hash, "%d:%s:%o:%d:", len(name), name, permissions, len(data))
	_, _ = hash.Write(data)
	if destination == "" {
		return nil
	}
	file := filepath.Join(destination, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	return os.WriteFile(file, data, permissions)
}
