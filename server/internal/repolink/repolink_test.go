package repolink

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseGitHubURLAcceptsOnlyHTTPSGitHubRepositories(t *testing.T) {
	for input, want := range map[string]string{
		"https://github.com/acme/app":         "https://github.com/acme/app",
		"https://github.com/acme/app.git":     "https://github.com/acme/app",
		"https://github.com/acme/app/":        "https://github.com/acme/app",
		"  https://GitHub.com/Acme/My.App_2 ": "https://github.com/acme/my.app_2",
		"HTTPS://GITHUB.COM/ACME/APP.git":     "https://github.com/acme/app",
		"https://github.com/a-b/c-d":          "https://github.com/a-b/c-d",
	} {
		repo, err := ParseGitHubURL(input)
		if err != nil || repo.URL() != want {
			t.Errorf("%q = %q, %v; want %q", input, repo.URL(), err, want)
		}
	}
	for _, input := range []string{
		"",
		"http://github.com/acme/app",
		"ssh://git@github.com/acme/app.git",
		"git@github.com:acme/app.git",
		"git://github.com/acme/app.git",
		"file:///srv/git/app.git",
		"/srv/git/app.git",
		"./app",
		"ext::sh -c touch% /tmp/pwned",
		"https://github.com/acme",
		"https://github.com/acme/app/tree/main",
		"https://github.com//acme/app",
		"https://user:token@github.com/acme/app",
		"https://github.com:443/acme/app",
		"https://github.com.evil.example/acme/app",
		"https://evil.example/github.com/acme/app",
		"https://gitlab.com/acme/app",
		"https://github.com/acme/app?tab=readme",
		"https://github.com/acme/app#readme",
		"https://github.com/acme/app%2F..",
		"https://github.com/-acme/app",
		"https://github.com/acme/..",
		"https://github.com/acme/.git",
		"https://github.com/acme/ap p",
		"https://github.com/acme/a\npp",
		"https://github.com/" + strings.Repeat("a", 40) + "/app",
	} {
		if _, err := ParseGitHubURL(input); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestSyncErrorsSayWhatToFix(t *testing.T) {
	repo := Repo{"acme", "app"}
	for stderr, want := range map[string]string{
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled":         "the server's git login can't read acme/app",
		"remote: Repository not found.\nfatal: repository 'https://github.com/acme/app/' not found":  "the server's git login can't read acme/app",
		"fatal: unable to access 'https://github.com/acme/app/': Could not resolve host: github.com": "the server can't reach GitHub",
		"fatal: something else broke": "clone acme/app: fatal: something else broke",
	} {
		err := describe("clone", repo, &gitError{stderr: stderr, err: errors.New("exit status 128")})
		if !errors.Is(err, ErrSync) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v; want %q", stderr, err, want)
		}
	}
	err := describe("fetch", repo, &gitError{timedOut: true, timeout: fetchTimeout})
	if !errors.Is(err, ErrSync) || !strings.Contains(err.Error(), "timed out after "+(5*time.Minute).String()) {
		t.Errorf("timeout: %v", err)
	}
}
