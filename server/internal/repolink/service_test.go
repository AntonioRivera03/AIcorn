package repolink

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	_ "modernc.org/sqlite"
)

// testDB is a migrated workspace database with two projects and a task, in a
// directory of its own: clones land in <that directory>/repos.
func testDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Test')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(1,1,'Open','open','gray','circle',1)`,
		`INSERT INTO project(id,name,workflow) VALUES(1,'One',1),(2,'Two',1)`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(1,1,'Tasks',1)`,
		`INSERT INTO task(id,checklist,stage,type,name,priority) SELECT 1,1,1,id,'Task','Medium' FROM task_type ORDER BY isDefault DESC,id LIMIT 1`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return db, filepath.Join(resolved, "repos")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// upstream stands in for GitHub: a bare repository, and a working copy that
// pushes to it.
type upstream struct{ bare, work string }

func newUpstream(t *testing.T) upstream {
	t.Helper()
	u := upstream{bare: filepath.Join(t.TempDir(), "app.git"), work: t.TempDir()}
	git(t, u.work, "init", "-q", "-b", "main")
	git(t, u.work, "init", "-q", "--bare", "-b", "main", u.bare)
	git(t, u.work, "remote", "add", "origin", u.bare)
	u.commit(t, "main", "first")
	u.commit(t, "old", "short-lived branch")
	return u
}

// commit adds a commit to branch (created from main if new), pushes it, and
// returns its hash.
func (u upstream) commit(t *testing.T, branch, message string) string {
	t.Helper()
	if branch != "main" {
		git(t, u.work, "checkout", "-q", "-B", branch)
		defer git(t, u.work, "checkout", "-q", "main")
	}
	if err := os.WriteFile(filepath.Join(u.work, "README.md"), []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, u.work, "add", "README.md")
	git(t, u.work, "commit", "-q", "-m", message)
	git(t, u.work, "push", "-q", "origin", branch)
	return git(t, u.work, "rev-parse", "HEAD")
}

// testService clones from local upstreams through the test hooks: remotes
// maps owner/name to a bare repository.
func testService(t *testing.T, db *sql.DB, remotes map[string]string) (*Service, *atomic.Int32) {
	t.Helper()
	var syncs atomic.Int32
	s := &Service{DB: db, protocols: "file", remote: func(repo Repo) string {
		syncs.Add(1)
		return remotes[repo.String()]
	}}
	t.Cleanup(s.Wait)
	return s, &syncs
}

func ptr[T any](v T) *T { return &v }

func linkOfficial(t *testing.T, s *Service, project int, url string) Status {
	t.Helper()
	status, err := s.Update(context.Background(), project, Patch{Mode: ptr(Official), URL: ptr(url)})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func remoteBranches(t *testing.T, root string) []string {
	t.Helper()
	return strings.Fields(git(t, root, "for-each-ref", "--format=%(refname:strip=2)", "refs/remotes/origin/"))
}

func TestOfficialLinkClonesThenFetchesTheLatestSource(t *testing.T) {
	ctx := context.Background()
	db, repos := testDB(t)
	u := newUpstream(t)
	s, _ := testService(t, db, map[string]string{"acme/app": u.bare})

	status := linkOfficial(t, s, 1, " https://GitHub.com/Acme/App.git ")
	if status.URL != "https://github.com/acme/app" || status.Mode != Official || !status.Linked {
		t.Fatalf("link not normalized: %+v", status)
	}
	source, err := s.SourceRepo(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repos, "1", "acme", "app")
	main := git(t, u.work, "rev-parse", "main")
	if source.Mode != Official || source.Root != root || source.Base != main {
		t.Fatalf("source = %+v; want root %s at %s", source, root, main)
	}
	if located, err := Locate(ctx, db, 1); err != nil || located.Root != root || located.Base != "" {
		t.Fatalf("Locate = %+v, %v", located, err)
	}
	// Agent worktrees inside the clone don't show up in its status.
	git(t, root, "check-ignore", "-q", ".worktrees/run-x/file")

	feature := u.commit(t, "feature", "new branch")
	main = u.commit(t, "main", "second")
	git(t, u.work, "push", "-q", "origin", "--delete", "old")
	if source, err = s.SourceRepo(ctx, 1); err != nil || source.Base != main {
		t.Fatalf("after fetch: %+v, %v; want base %s", source, err, main)
	}
	branches := strings.Join(remoteBranches(t, root), " ")
	if !strings.Contains(branches, "origin/feature") || strings.Contains(branches, "origin/old") {
		t.Fatalf("fetch did not add and prune branches: %s", branches)
	}
	if got := git(t, root, "rev-parse", "origin/feature"); got != feature {
		t.Fatalf("origin/feature = %s; want %s", got, feature)
	}
	status, err = s.Status(ctx, 1)
	if err != nil || !status.Cloned || status.Syncing || status.FetchedAt == nil || status.Error != "" {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

func TestFetchFollowsTheRemoteDefaultBranch(t *testing.T) {
	ctx := context.Background()
	db, repos := testDB(t)
	u := newUpstream(t)
	s, _ := testService(t, db, map[string]string{"acme/app": u.bare})
	linkOfficial(t, s, 1, "https://github.com/acme/app")
	if _, err := s.SourceRepo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	dev := u.commit(t, "dev", "the new default")
	git(t, u.bare, "symbolic-ref", "HEAD", "refs/heads/dev")
	source, err := s.SourceRepo(ctx, 1)
	if err != nil || source.Base != dev {
		t.Fatalf("source = %+v, %v; want base %s", source, err, dev)
	}
	root := filepath.Join(repos, "1", "acme", "app")
	if got := git(t, root, "symbolic-ref", "refs/remotes/origin/HEAD"); got != "refs/remotes/origin/dev" {
		t.Fatalf("origin/HEAD = %s", got)
	}
}

func TestSyncIsSerializedPerProject(t *testing.T) {
	ctx := context.Background()
	db, _ := testDB(t)
	u := newUpstream(t)
	s, syncs := testService(t, db, map[string]string{"acme/one": u.bare, "acme/two": u.bare})
	for project, url := range map[int]string{1: "https://github.com/acme/one", 2: "https://github.com/acme/two"} {
		if _, err := saveAndLoad(t, db, project, Link{Mode: Official, URL: url}); err != nil {
			t.Fatal(err)
		}
	}

	unlock, err := s.lock(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	// While project 1 is busy, a caller that can't wait is told so...
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = s.SourceRepo(short, 1)
	cancel()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy while the project is locked, got %v", err)
	}
	// ...and another project isn't held up.
	if _, err = s.SourceRepo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	// Callers that arrive while project 1 is busy share one clone.
	var wg sync.WaitGroup
	results := make([]Source, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = s.SourceRepo(ctx, 1)
		}()
	}
	time.Sleep(200 * time.Millisecond)
	unlock()
	wg.Wait()
	for i := range results {
		if errs[i] != nil || results[i] != results[0] || results[i].Base == "" {
			t.Fatalf("caller %d: %+v, %v", i, results[i], errs[i])
		}
	}
	if got := syncs.Load(); got != 2 { // one clone per project
		t.Fatalf("git synced %d times; want 2", got)
	}
}

func saveAndLoad(t *testing.T, db *sql.DB, project int, link Link) (Link, error) {
	t.Helper()
	if err := saveLink(context.Background(), db, project, link); err != nil {
		return Link{}, err
	}
	return loadLink(context.Background(), db, project)
}

func TestFailedCloneLeavesNothingBehindAndSaysWhy(t *testing.T) {
	ctx := context.Background()
	db, repos := testDB(t)
	u := newUpstream(t)
	remotes := map[string]string{"acme/app": filepath.Join(t.TempDir(), "missing.git")}
	s, _ := testService(t, db, remotes)
	linkOfficial(t, s, 1, "https://github.com/acme/app")
	_, err := s.SourceRepo(ctx, 1)
	if !errors.Is(err, ErrSync) || !strings.Contains(err.Error(), "clone acme/app") {
		t.Fatalf("clone error = %v", err)
	}
	s.Wait()
	status, err := s.Status(ctx, 1)
	if err != nil || status.Cloned || status.FetchedAt != nil || !strings.Contains(status.Error, "acme/app") {
		t.Fatalf("status after a failed clone = %+v, %v", status, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(repos, "1")); len(entries) != 0 {
		t.Fatalf("a failed clone left %d entries behind", len(entries))
	}
	// Fixing access and fetching again recovers.
	remotes["acme/app"] = u.bare
	if status, err = s.Fetch(ctx, 1); err != nil || !status.Cloned || status.Error != "" || status.FetchedAt == nil {
		t.Fatalf("fetch now = %+v, %v", status, err)
	}
}

func TestProductionOnlyUsesHTTPS(t *testing.T) {
	db, repos := testDB(t)
	u := newUpstream(t)
	s := &Service{DB: db} // no test hooks
	if s.remoteURL(Repo{"acme", "app"}) != "https://github.com/acme/app" {
		t.Fatal("production clones something other than the validated GitHub URL")
	}
	// Even handed a local path directly, git refuses the transport.
	err := s.clone(context.Background(), filepath.Join(repos, "1", "acme", "app"), u.bare)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("local clone was not refused: %v", err)
	}
}

func TestLinkChangesRemoveTheOldClone(t *testing.T) {
	ctx := context.Background()
	db, repos := testDB(t)
	u := newUpstream(t)
	s, _ := testService(t, db, map[string]string{"acme/one": u.bare, "acme/two": u.bare})
	linkOfficial(t, s, 1, "https://github.com/acme/one")
	one, err := s.SourceRepo(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Not while an agent works in the clone: its branch lives there.
	if _, err = db.Exec(`INSERT INTO agent_job(id,task,status,requestJson) VALUES(1,1,'running',json_object('repoPath',?))`, one.Root); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(ctx, 1, Patch{URL: ptr("https://github.com/acme/two")}); !errors.Is(err, ErrBusy) {
		t.Fatalf("relinked under a running agent: %v", err)
	}
	if link, _ := loadLink(ctx, db, 1); link.URL != "https://github.com/acme/one" {
		t.Fatalf("refused change was saved: %+v", link)
	}
	if _, err = db.Exec(`UPDATE agent_job SET status='completed' WHERE id=1`); err != nil {
		t.Fatal(err)
	}

	// A new URL deletes the old clone and clones the new one.
	status := linkOfficial(t, s, 1, "https://github.com/acme/two")
	s.Wait()
	two := filepath.Join(repos, "1", "acme", "two")
	if _, err = os.Stat(one.Root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old clone kept: %v", err)
	}
	if _, err = os.Stat(filepath.Join(two, ".git")); err != nil || status.URL != "https://github.com/acme/two" {
		t.Fatalf("new link not cloned on linking: %v", err)
	}

	// Switching to Personal deletes the clone but keeps the URL for later.
	personal := t.TempDir()
	git(t, personal, "init", "-q")
	status, err = s.Update(ctx, 1, Patch{Mode: ptr(Personal), Path: ptr(personal)})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if _, err = os.Stat(filepath.Join(repos, "1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clone kept after switching to Personal: %v", err)
	}
	if status.Mode != Personal || status.URL != "https://github.com/acme/two" || status.Path != personal || !status.Linked || status.Cloned {
		t.Fatalf("status after switching = %+v", status)
	}
}

func TestPruneRemovesDeletedProjectsAndInterruptedClones(t *testing.T) {
	ctx := context.Background()
	db, repos := testDB(t)
	u := newUpstream(t)
	s, _ := testService(t, db, map[string]string{"acme/app": u.bare})
	linkOfficial(t, s, 2, "https://github.com/acme/app")
	source, err := s.SourceRepo(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	stray := []string{
		filepath.Join(repos, "2", ".clone-123", "README.md"),  // an interrupted clone
		filepath.Join(repos, "2", "acme", "old", "README.md"), // an earlier link
		filepath.Join(repos, "2", "other", "app", "README.md"),
		filepath.Join(repos, "77", "acme", "app", "README.md"), // a deleted project
	}
	for _, file := range stray {
		if err = os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Startup cleans up after a server that stopped mid-cleanup.
	started, _ := testService(t, db, nil)
	started.Start(ctx)
	started.Wait()
	for _, file := range stray {
		if _, err = os.Stat(filepath.Dir(file)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s survived startup cleanup", file)
		}
	}
	if _, err = os.Stat(filepath.Join(source.Root, ".git")); err != nil {
		t.Fatalf("the current clone was removed: %v", err)
	}

	// Deleting the project removes its clone.
	if _, err = db.Exec(`DELETE FROM project WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	s.Forget(2)
	s.Wait()
	if _, err = os.Stat(filepath.Join(repos, "2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted project's clone kept: %v", err)
	}
	var synced int
	if err = db.QueryRow(`SELECT COUNT(*) FROM repository_sync WHERE project=2`).Scan(&synced); err != nil || synced != 0 {
		t.Fatalf("sync status outlived its project: %d, %v", synced, err)
	}
}

func TestPersonalLinksResolveToTheCheckoutRoot(t *testing.T) {
	ctx := context.Background()
	db, _ := testDB(t)
	s, _ := testService(t, db, nil)
	if _, err := Locate(ctx, db, 1); !errors.Is(err, ErrNotLinked) {
		t.Fatalf("a project without a link resolved: %v", err)
	}
	checkout := t.TempDir()
	git(t, checkout, "init", "-q")
	if err := os.MkdirAll(filepath.Join(checkout, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, 1, Patch{Mode: ptr(Personal), Path: ptr(filepath.Join(checkout, "server") + " ")}); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.EvalSymlinks(checkout)
	for name, resolve := range map[string]func(context.Context, int) (Source, error){
		"Locate":     func(ctx context.Context, p int) (Source, error) { return Locate(ctx, db, p) },
		"SourceRepo": s.SourceRepo,
	} {
		source, err := resolve(ctx, 1)
		if err != nil || source.Mode != Personal || source.Root != root || source.Base != "" {
			t.Fatalf("%s = %+v, %v; want the checkout root %s", name, source, err, root)
		}
	}

	// An unchanged link isn't written, so the project keeps its modified time.
	if _, err := db.Exec(`UPDATE project SET timeModified='2020-01-01 00:00:00' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, 1, Patch{Mode: ptr(Personal), Path: ptr(filepath.Join(checkout, "server"))}); err != nil {
		t.Fatal(err)
	}
	var modified string
	if err := db.QueryRow(`SELECT date(timeModified) FROM project WHERE id=1`).Scan(&modified); err != nil || modified != "2020-01-01" {
		t.Fatalf("no-op update bumped timeModified to %s, %v", modified, err)
	}

	for _, patch := range []Patch{
		{Path: ptr("relative/path")},
		{Mode: ptr(Mode("gitlab"))},
		{URL: ptr("git@github.com:acme/app.git")},
		{URL: ptr("file:///srv/app.git")},
	} {
		if _, err := s.Update(ctx, 1, patch); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", patch, err)
		}
	}
	if _, err := s.Update(ctx, 1, Patch{Path: ptr(filepath.Join(checkout, "missing"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := Locate(ctx, db, 1); !errors.Is(err, ErrRepoInvalid) {
		t.Fatalf("a missing folder resolved: %v", err)
	}
	status, err := s.Update(ctx, 1, Patch{Path: ptr("")})
	if err != nil || status.Linked {
		t.Fatalf("clearing the path = %+v, %v", status, err)
	}
	if _, err := Locate(ctx, db, 1); !errors.Is(err, ErrNotLinked) {
		t.Fatalf("an empty Personal link resolved: %v", err)
	}
}
