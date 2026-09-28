package repolink

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// syncClone makes root an up-to-date clone of repo and returns the commit of
// the remote's default branch.
func (s *Service) syncClone(ctx context.Context, root string, repo Repo) (string, error) {
	remote := s.remoteURL(repo)
	_, err := os.Stat(filepath.Join(root, ".git"))
	switch {
	case err == nil:
		if err = s.fetch(ctx, root, remote); err != nil {
			return "", describe("fetch", repo, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// Anything at root without a .git isn't a clone, only in the way.
		if err = os.RemoveAll(root); err != nil {
			return "", err
		}
		if err = s.clone(ctx, root, remote); err != nil {
			return "", describe("clone", repo, err)
		}
	default:
		return "", err
	}
	base, err := s.git(ctx, localTimeout, root, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %s has no default branch to start from; push a commit to it first", ErrSync, repo)
	}
	return strings.TrimSpace(base), nil
}

// clone clones into a temporary directory beside the final one and renames it
// into place, so an interrupted clone never looks like a finished one.
func (s *Service) clone(ctx context.Context, root, remote string) error {
	parent := filepath.Dir(filepath.Dir(root)) // <repos>/<project>
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".clone-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if _, err = s.git(ctx, cloneTimeout, parent, "clone", "--quiet", "--", remote, temp); err != nil {
		return err
	}
	// Agent runs add worktrees under <clone>/.worktrees; keep them out of the
	// clone's own status, as a user's checkout does with .gitignore.
	info := filepath.Join(temp, ".git", "info")
	if err = os.MkdirAll(info, 0o755); err != nil {
		return err
	}
	exclude, err := os.OpenFile(filepath.Join(info, "exclude"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err = exclude.WriteString("\n/.worktrees/\n"); err != nil {
		exclude.Close()
		return err
	}
	if err = exclude.Close(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return err
	}
	return os.Rename(temp, root)
}

// fetch updates the remote-tracking branches from the validated URL itself,
// never from the clone's configured remote, then follows the remote's default
// branch in case it changed.
func (s *Service) fetch(ctx context.Context, root, remote string) error {
	if _, err := s.git(ctx, fetchTimeout, root, "fetch", "--prune", "--quiet", "--", remote, "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	out, err := s.git(ctx, fetchTimeout, root, "ls-remote", "--symref", "--", remote, "HEAD")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		branch, ok := strings.CutPrefix(line, "ref: refs/heads/")
		if !ok {
			continue
		}
		branch, ok = strings.CutSuffix(branch, "\tHEAD")
		if !ok {
			continue
		}
		target := "refs/remotes/origin/" + branch
		if _, err = s.git(ctx, localTimeout, root, "check-ref-format", target); err != nil {
			return err
		}
		_, err = s.git(ctx, localTimeout, root, "symbolic-ref", "refs/remotes/origin/HEAD", target)
		return err
	}
	return nil // an empty repository has no default branch yet
}
