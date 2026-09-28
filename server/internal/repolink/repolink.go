// Package repolink resolves where a project's code lives. A project links its
// repository one of two ways:
//
//   - Personal: a local checkout the user owns and keeps up to date. Aycorn
//     works from it as it is.
//   - Official: a GitHub repository Aycorn manages. Aycorn clones it into the
//     workspace's own directory, <workspace dir>/repos/<project>/<owner>/<repo>,
//     with the server machine's git login, and fetches before every
//     environment build and agent run. It never pushes.
//
// Every consumer that needs a project's code (agent runs, branch environments,
// Conductor's repository check) goes through Locate or Service.SourceRepo, the
// one place that knows about the two modes. See Documentation/repo-linking.md.
package repolink

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

type Mode string

const (
	Personal Mode = "personal"
	Official Mode = "official"
)

var (
	ErrInvalid     = errors.New("invalid repository link")
	ErrNotLinked   = errors.New("project has no repository linked; link one in Project settings → General")
	ErrRepoInvalid = errors.New("linked repo folder is not a valid git repository")
	// ErrSync means cloning or fetching an Official link failed; its message
	// says what to fix.
	ErrSync = errors.New("repository sync failed")
	// ErrBusy means the clone can't change right now: an agent is working in
	// it, or a clone or fetch is still running.
	ErrBusy = errors.New("repository busy")
)

// Link is how a project reaches its code, as the user configured it. Path and
// URL are both kept, so switching modes back and forth loses neither.
type Link struct {
	Mode Mode   `json:"mode"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

// Linked reports whether the chosen mode has somewhere to work from.
func (l Link) Linked() bool {
	return (l.Mode == Personal && l.Path != "") || (l.Mode == Official && l.URL != "")
}

// Source is the checkout a project's code is read from and agents work in.
type Source struct {
	Mode Mode
	// Root is the repository root: the user's checkout, or Aycorn's clone.
	Root string
	// Base is the commit new agent runs start from. Empty means the
	// checkout's HEAD (Personal). Official runs start from the remote's
	// default branch as of the latest fetch; only SourceRepo sets it.
	Base string
}

// Repo is a GitHub repository.
type Repo struct{ Owner, Name string }

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// URL is the normalized form Aycorn stores and clones.
func (r Repo) URL() string { return "https://github.com/" + r.Owner + "/" + r.Name }

// GitHub account names are alphanumeric with hyphens; repository names also
// allow '.' and '_'. Both are case-insensitive, so they're kept lowercase (as
// task GitHub links are).
var (
	githubOwner = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,38}$`)
	githubName  = regexp.MustCompile(`^[a-z0-9._-]{1,100}$`)
)

// ParseGitHubURL accepts only https://github.com/<owner>/<repo>, optionally
// ending in .git or a slash. Every other form — ssh, git@, file://, ext::, a
// local path, a port, credentials, a query — is rejected, so what git clones
// is always a plain HTTPS GitHub address.
func ParseGitHubURL(raw string) (Repo, error) {
	invalid := fmt.Errorf("%w: use a GitHub repository URL like https://github.com/owner/repo", ErrInvalid)
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 300 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return Repo{}, invalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || strings.ToLower(u.Host) != "github.com" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return Repo{}, invalid
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	if len(parts) != 2 {
		return Repo{}, invalid
	}
	repo := Repo{Owner: strings.ToLower(parts[0]), Name: strings.TrimSuffix(strings.ToLower(parts[1]), ".git")}
	if !githubOwner.MatchString(repo.Owner) || !githubName.MatchString(repo.Name) || repo.Name == "." || repo.Name == ".." {
		return Repo{}, invalid
	}
	return repo, nil
}
