package repolink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Every git call gets a deadline. Clones and fetches talk to GitHub; the rest
// only read the local clone.
const (
	cloneTimeout = 20 * time.Minute
	fetchTimeout = 5 * time.Minute
	localTimeout = 30 * time.Second
)

// gitError keeps git's own message so describe can turn it into advice.
type gitError struct {
	stderr   string
	timeout  time.Duration
	timedOut bool
	err      error
}

func (e *gitError) Error() string {
	if e.timedOut {
		return fmt.Sprintf("timed out after %s", e.timeout)
	}
	if line := lastLine(e.stderr); line != "" {
		return line
	}
	return e.err.Error()
}

func (e *gitError) Unwrap() error { return e.err }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// git runs git with an argument list (never a shell) under a timeout. It never
// prompts: a missing login fails instead of waiting on a terminal. Only the
// allowed transports can be used, so a clone or fetch can't be pointed at
// file://, ext:: or ssh even by a changed git config. Cancelling interrupts
// git rather than killing it, so it removes its lock files.
func (s *Service) git(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ALLOW_PROTOCOL="+s.allowedProtocols())
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 10 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &gitError{stderr: stderr.String(), timeout: timeout, timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded), err: err}
	}
	return stdout.String(), nil
}

// describe turns a failed clone or fetch into an ErrSync the user can act on.
func describe(action string, repo Repo, err error) error {
	var ge *gitError
	if !errors.As(err, &ge) {
		return fmt.Errorf("%w: %s %s: %v", ErrSync, action, repo, err)
	}
	if ge.timedOut {
		return fmt.Errorf("%w: %s %s timed out after %s; try again", ErrSync, action, repo, ge.timeout)
	}
	message := strings.ToLower(ge.stderr)
	for _, sign := range []string{"authentication failed", "could not read username", "could not read password", "terminal prompts disabled", "repository not found", "permission denied", "access denied", "the requested url returned error: 403", "the requested url returned error: 401"} {
		if strings.Contains(message, sign) {
			return fmt.Errorf("%w: the server's git login can't read %s. Sign in on the server machine with `gh auth login` (or set up a git credential helper) as an account that can read the repository, then fetch again", ErrSync, repo)
		}
	}
	for _, sign := range []string{"could not resolve host", "failed to connect", "connection timed out", "network is unreachable", "connection refused"} {
		if strings.Contains(message, sign) {
			return fmt.Errorf("%w: the server can't reach GitHub to %s %s: %s", ErrSync, action, repo, ge)
		}
	}
	return fmt.Errorf("%w: %s %s: %s", ErrSync, action, repo, ge)
}
