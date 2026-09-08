package worktree

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrBranchUnavailable = errors.New("branch unavailable")
var ErrMergeBlocked = errors.New("merge blocked")

// Serialize app-initiated merges. Git's own locks still protect against other processes.
var mergeMu sync.Mutex

type BranchState struct {
	Commit     string   `json:"commit"`
	Workspace  string   `json:"workspace"`
	Dirty      bool     `json:"dirty"`
	MergedInto []string `json:"mergedInto"`
}
type MergePreview struct {
	Target        string   `json:"target"`
	Targets       []string `json:"targets"`
	Token         string   `json:"token"`
	Commits       int      `json:"commits"`
	Uncommitted   bool     `json:"uncommitted"`
	Files         []string `json:"files"`
	Diff          string   `json:"diff"`
	Blocked       string   `json:"blocked,omitempty"`
	AlreadyMerged bool     `json:"alreadyMerged"`
	source        string
	targetCommit  string
	tree          string
	workspace     string
}
type MergeResult struct {
	Target  string `json:"target"`
	Commit  string `json:"commit"`
	Output  string `json:"output"`
	Warning string `json:"warning,omitempty"`
}

func localRef(ctx context.Context, root, branch string) (string, error) {
	if root == "" || !filepath.IsAbs(root) || branch == "" || strings.HasPrefix(branch, "-") || runGit(ctx, root, "check-ref-format", "refs/heads/"+branch) != nil {
		return "", fmt.Errorf("%w: invalid local branch", ErrBranchUnavailable)
	}
	out, err := gitOutput(ctx, root, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %s no longer exists in %s", ErrBranchUnavailable, branch, root)
	}
	return strings.TrimSpace(out), nil
}

// Use porcelain's NUL format so spaces, tabs and newlines in paths survive.
func branchWorkspace(ctx context.Context, root, branch string) (string, error) {
	raw, err := gitOutput(ctx, root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", err
	}
	var path string
	for _, field := range strings.Split(raw, "\x00") {
		if strings.HasPrefix(field, "worktree ") {
			path = strings.TrimPrefix(field, "worktree ")
		}
		if field == "branch refs/heads/"+branch {
			return path, nil
		}
	}
	return "", nil
}

func workspaceStatus(ctx context.Context, path string) (string, error) {
	return gitOutput(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
}

func InspectBranch(ctx context.Context, root, branch, base string) (*BranchState, error) {
	commit, err := localRef(ctx, root, branch)
	if err != nil {
		return nil, err
	}
	path, err := branchWorkspace(ctx, root, branch)
	if err != nil {
		return nil, err
	}
	state := &BranchState{Commit: commit, Workspace: path, MergedInto: []string{}}
	if path != "" {
		status, err := workspaceStatus(ctx, path)
		if err != nil {
			return nil, err
		}
		state.Dirty = status != ""
	}
	// A newly created empty branch is not a delivered change.
	if !state.Dirty && commit != base {
		raw, err := gitOutput(ctx, root, "for-each-ref", "--format=%(refname:strip=2)", "--contains="+commit, "refs/heads/")
		if err != nil {
			return nil, err
		}
		for _, name := range strings.Fields(raw) {
			if name != branch {
				state.MergedInto = append(state.MergedInto, name)
			}
		}
	}
	return state, nil
}

func workspaceReady(ctx context.Context, path string, clean bool) error {
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		raw, err := gitOutput(ctx, path, "rev-parse", "--git-path", name)
		if err != nil {
			return err
		}
		p := strings.TrimSpace(raw)
		if !filepath.IsAbs(p) {
			p = filepath.Join(path, p)
		}
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%w: finish the Git operation in %s first", ErrMergeBlocked, path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	status, err := workspaceStatus(ctx, path)
	if err != nil {
		return err
	}
	if clean && status != "" {
		return fmt.Errorf("%w: commit or stash changes in the destination workspace first: %s", ErrMergeBlocked, path)
	}
	return nil
}

// A private index lets a review include staged, unstaged and new files without
// changing the agent's real staging area. The tree hash also detects stale reviews.
func snapshotTree(ctx context.Context, root, path, commit string) (string, error) {
	if path == "" {
		raw, err := gitOutput(ctx, root, "rev-parse", commit+"^{tree}")
		return strings.TrimSpace(raw), err
	}
	if err := workspaceReady(ctx, path, false); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp("", "aycorn-review-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = path
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "GIT_INDEX_FILE=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+filepath.Join(temp, "index"))
		raw, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(raw)))
		}
		return strings.TrimSpace(string(raw)), nil
	}
	indexTree, err := gitOutput(ctx, path, "write-tree")
	if err != nil {
		return "", fmt.Errorf("%w: resolve the agent workspace index first: %v", ErrMergeBlocked, err)
	}
	if _, err = run("read-tree", strings.TrimSpace(indexTree)); err != nil {
		return "", err
	}
	if _, err = run("add", "-A", "--", "."); err != nil {
		return "", err
	}
	return run("write-tree")
}

func PreviewMerge(ctx context.Context, root, branch, target string) (*MergePreview, error) {
	state, err := InspectBranch(ctx, root, branch, "")
	if err != nil {
		return nil, err
	}
	raw, err := gitOutput(ctx, root, "for-each-ref", "--format=%(refname:strip=2)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	p := &MergePreview{Targets: []string{}, Files: []string{}, source: state.Commit, workspace: state.Workspace, Uncommitted: state.Dirty}
	for _, name := range strings.Fields(raw) {
		if name != branch {
			p.Targets = append(p.Targets, name)
		}
	}
	if target == "" {
		current, _ := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
		for _, candidate := range []string{strings.TrimSpace(current), "main", "master"} {
			for _, name := range p.Targets {
				if target == "" && name == candidate {
					target = name
				}
			}
		}
		if target == "" && len(p.Targets) > 0 {
			target = p.Targets[0]
		}
	}
	p.Target = target
	if target == "" {
		p.Blocked = "Create a destination branch in this repository first."
		return p, nil
	}
	if target == branch {
		return nil, fmt.Errorf("%w: choose a different destination branch", ErrMergeBlocked)
	}
	p.targetCommit, err = localRef(ctx, root, target)
	if err != nil {
		return nil, err
	}
	targetPath, err := branchWorkspace(ctx, root, target)
	if err != nil {
		return nil, err
	}
	if targetPath != "" {
		if err := workspaceReady(ctx, targetPath, true); err != nil {
			p.Blocked = err.Error()
		}
	}
	p.tree, err = snapshotTree(ctx, root, state.Workspace, state.Commit)
	if err != nil {
		return nil, err
	}
	headTree, err := gitOutput(ctx, root, "rev-parse", state.Commit+"^{tree}")
	if err != nil {
		return nil, err
	}
	if state.Dirty && p.tree == strings.TrimSpace(headTree) {
		p.Blocked = "The workspace has changes that cannot be committed here (such as changes inside a submodule). Resolve them in the terminal first."
	}
	base, err := gitOutput(ctx, root, "merge-base", p.targetCommit, state.Commit)
	if err != nil {
		p.Blocked = "These branches do not share Git history."
		return p, nil
	}
	base = strings.TrimSpace(base)
	raw, err = gitOutput(ctx, root, "rev-list", "--count", p.targetCommit+".."+state.Commit)
	if err != nil {
		return nil, err
	}
	p.Commits, err = strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	p.AlreadyMerged = p.Commits == 0 && !p.Uncommitted
	raw, err = gitOutput(ctx, root, "diff", "--name-only", "-z", base, p.tree, "--")
	if err != nil {
		return nil, err
	}
	for _, file := range strings.Split(raw, "\x00") {
		if file != "" {
			p.Files = append(p.Files, file)
		}
	}
	p.Diff, err = gitOutput(ctx, root, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--stat", "--patch", base, p.tree, "--")
	if err != nil {
		return nil, err
	}
	if len(p.Diff) > 256*1024 {
		p.Diff = p.Diff[:256*1024] + "\n… Diff truncated. Inspect the workspace for the full change.\n"
	}
	p.Token = fmt.Sprintf("%x", sha256.Sum256([]byte(branch+"\x00"+target+"\x00"+p.source+"\x00"+p.targetCommit+"\x00"+p.tree)))
	return p, nil
}

func MergeBranch(ctx context.Context, root, branch, target, token string, taskID, jobID int) (_ *MergeResult, returnErr error) {
	mergeMu.Lock()
	defer mergeMu.Unlock()
	p, err := PreviewMerge(ctx, root, branch, target)
	if err != nil {
		return nil, err
	}
	if p.Blocked != "" {
		return nil, fmt.Errorf("%w: %s", ErrMergeBlocked, p.Blocked)
	}
	if token == "" || token != p.Token {
		return nil, fmt.Errorf("%w: branches or files changed since review. Refresh the preview before merging", ErrMergeBlocked)
	}
	if p.AlreadyMerged {
		return &MergeResult{Target: target, Commit: p.targetCommit, Output: "Already merged."}, nil
	}
	if p.Uncommitted {
		head, err := gitOutput(ctx, p.workspace, "symbolic-ref", "--quiet", "HEAD")
		if err != nil || strings.TrimSpace(head) != "refs/heads/"+branch {
			return nil, fmt.Errorf("%w: agent checkout changed. Review again", ErrMergeBlocked)
		}
		// Verify identity before staging, so missing Git configuration leaves the index alone.
		if _, err := gitOutput(ctx, p.workspace, "var", "GIT_AUTHOR_IDENT"); err != nil {
			return nil, fmt.Errorf("%w: configure Git user.name and user.email before committing: %v", ErrMergeBlocked, err)
		}
		if err := runGit(ctx, p.workspace, "add", "-A", "--", "."); err != nil {
			return nil, err
		}
		if err := runGit(ctx, p.workspace, "commit", "-m", fmt.Sprintf("Task #%d: agent run #%d", taskID, jobID)); err != nil {
			return nil, fmt.Errorf("%w: could not commit agent changes: %v", ErrMergeBlocked, err)
		}
		p.source, err = localRef(ctx, root, branch)
		if err != nil {
			return nil, err
		}
		committedTree, err := gitOutput(ctx, root, "rev-parse", p.source+"^{tree}")
		if err != nil {
			return nil, err
		}
		status, err := workspaceStatus(ctx, p.workspace)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(committedTree) != p.tree || status != "" {
			return nil, fmt.Errorf("%w: files changed during the commit. The commit is preserved; review again before merging", ErrMergeBlocked)
		}
	}
	// Trial merge in a disposable detached worktree: conflicts never touch the
	// destination's files or index, and a failed merge preserves the source branch.
	temp, err := os.MkdirTemp("", "aycorn-merge-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	path := filepath.Join(temp, "worktree")
	if err := runGit(ctx, root, "worktree", "add", "--detach", path, p.targetCommit); err != nil {
		return nil, err
	}
	result := &MergeResult{Target: target}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := runGit(cleanup, root, "worktree", "remove", "--force", path); err != nil {
			message := "Temporary merge workspace cleanup failed: " + err.Error()
			if returnErr != nil {
				returnErr = errors.Join(returnErr, errors.New(message))
			} else {
				result.Warning = message
			}
		}
	}()
	output, err := gitOutput(ctx, path, "merge", "--ff", "--no-edit", "--no-autostash", "-m", fmt.Sprintf("Merge %s into %s (task #%d)", branch, target, taskID), p.source)
	if err != nil {
		conflicts, readErr := gitOutput(ctx, path, "diff", "--name-only", "--diff-filter=U")
		if readErr == nil && strings.TrimSpace(conflicts) != "" {
			return nil, fmt.Errorf("%w: conflicts in %s. Resolve them on %s in the terminal, then try again. Agent changes are preserved; %s was not changed", ErrMergeBlocked, strings.TrimSpace(conflicts), branch, target)
		}
		return nil, fmt.Errorf("%w: %v. Agent changes are preserved; %s was not changed", ErrMergeBlocked, err, target)
	}
	merged, err := gitOutput(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	result.Commit, result.Output = strings.TrimSpace(merged), output
	current, err := localRef(ctx, root, target)
	if err != nil {
		return nil, err
	}
	if current != p.targetCommit {
		return nil, fmt.Errorf("%w: destination moved during merge. Review again", ErrMergeBlocked)
	}
	destination, err := branchWorkspace(ctx, root, target)
	if err != nil {
		return nil, err
	}
	if destination == "" {
		err = runGit(ctx, root, "update-ref", "-m", "Aycorn: merge "+branch, "refs/heads/"+target, result.Commit, p.targetCommit)
	} else {
		if err := workspaceReady(ctx, destination, true); err != nil {
			return nil, err
		}
		// Never checkout another branch in the user's workspace.
		head, headErr := gitOutput(ctx, destination, "symbolic-ref", "--quiet", "HEAD")
		if headErr != nil || strings.TrimSpace(head) != "refs/heads/"+target {
			return nil, fmt.Errorf("%w: destination checkout changed. Review again", ErrMergeBlocked)
		}
		err = runGit(ctx, destination, "merge", "--ff-only", "--no-autostash", "--no-overwrite-ignore", result.Commit)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: could not update destination: %v", ErrMergeBlocked, err)
	}
	return result, nil
}
