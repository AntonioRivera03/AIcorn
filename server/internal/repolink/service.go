package repolink

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

// Locate returns where a project's code is without touching the network: the
// root of the Personal checkout, or where the Official clone lives, cloned yet
// or not. Use it to identify the repository (as a run's request records it)
// and Service.SourceRepo to work from it.
func Locate(ctx context.Context, db *sql.DB, project int) (Source, error) {
	link, err := loadLink(ctx, db, project)
	if err != nil {
		return Source{}, err
	}
	return locate(ctx, db, project, link)
}

func locate(ctx context.Context, db *sql.DB, project int, link Link) (Source, error) {
	switch link.Mode {
	case Personal:
		if link.Path == "" {
			return Source{}, ErrNotLinked
		}
		ctx, cancel := context.WithTimeout(ctx, localTimeout)
		defer cancel()
		root, err := worktree.RepoRootFromDir(ctx, link.Path)
		if err != nil {
			return Source{}, fmt.Errorf("%w: %v", ErrRepoInvalid, err)
		}
		return Source{Mode: Personal, Root: root}, nil
	case Official:
		root, err := officialRoot(ctx, db, project, link)
		if err != nil {
			return Source{}, err
		}
		return Source{Mode: Official, Root: root}, nil
	}
	return Source{}, ErrNotLinked
}

// officialRoot is where an Official link's clone lives, or ErrNotLinked.
func officialRoot(ctx context.Context, db *sql.DB, project int, link Link) (string, error) {
	if link.Mode != Official || link.URL == "" {
		return "", ErrNotLinked
	}
	repo, err := ParseGitHubURL(link.URL)
	if err != nil {
		return "", err
	}
	repos, err := reposDir(ctx, db)
	if err != nil {
		return "", err
	}
	return cloneRoot(repos, project, repo), nil
}

// Resolver syncs and returns a project's repository. *Service implements it;
// consumers depend on the interface so their tests can supply a checkout.
type Resolver interface {
	SourceRepo(ctx context.Context, project int) (Source, error)
}

// Service owns a workspace's Official clones: it clones and fetches them, one
// git operation per project at a time, and removes them when their link goes
// away. The workspace runtime builds one and shares it, so every caller is
// serialized by the same per-project lock.
type Service struct {
	DB *sql.DB

	mu      sync.Mutex
	ctx     context.Context
	wg      sync.WaitGroup
	locks   map[int]chan struct{}
	flights map[flightKey]*flight

	// Test hooks, set only by this package's tests. Production clones the
	// validated https://github.com URL over https alone; tests point remote at
	// a local bare repository and allow the file transport.
	remote    func(Repo) string
	protocols string
}

type flightKey struct {
	project int
	url     string
}

// flight is one clone-or-fetch, shared by everyone who asks while it runs.
type flight struct {
	done   chan struct{}
	source Source
	err    error
}

func (s *Service) remoteURL(repo Repo) string {
	if s.remote != nil {
		return s.remote(repo)
	}
	return repo.URL()
}

func (s *Service) allowedProtocols() string {
	if s.protocols != "" {
		return s.protocols
	}
	return "https"
}

// Start sets the context background clones, fetches and cleanups run under,
// then removes clones whose project or link is gone (left behind when the
// server stopped mid-cleanup).
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	s.background(s.pruneAll)
}

// Wait returns once background work has stopped. Cancel Start's context first.
func (s *Service) Wait() { s.wg.Wait() }

// background runs work under the service's context. It reports false, and
// runs nothing, once the workspace is stopping.
func (s *Service) background(work func(context.Context)) bool {
	s.mu.Lock()
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		s.mu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		work(ctx)
	}()
	return true
}

// lock serializes git work and cleanup per project.
func (s *Service) lock(ctx context.Context, project int) (func(), error) {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = map[int]chan struct{}{}
	}
	held := s.locks[project]
	if held == nil {
		held = make(chan struct{}, 1)
		s.locks[project] = held
	}
	s.mu.Unlock()
	select {
	case held <- struct{}{}:
		return func() { <-held }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SourceRepo returns the checkout to work from, synced first. A Personal link
// is returned as it is. For an Official link it clones the repository if
// Aycorn hasn't yet, fetches (git fetch --prune), and resolves the remote's
// default branch as the Base for new runs.
//
// The clone or fetch runs in the background and is shared by concurrent
// callers for the same link. A caller whose ctx ends first gets ErrBusy, and
// the work carries on for the next caller.
func (s *Service) SourceRepo(ctx context.Context, project int) (Source, error) {
	link, err := loadLink(ctx, s.DB, project)
	if err != nil {
		return Source{}, err
	}
	if link.Mode != Official {
		return locate(ctx, s.DB, project, link)
	}
	if _, err = officialRoot(ctx, s.DB, project, link); err != nil {
		return Source{}, err
	}
	f := s.startSync(project, link.URL)
	select {
	case <-f.done:
		return f.source, f.err
	case <-ctx.Done():
		return Source{}, fmt.Errorf("%w: Aycorn is still cloning or fetching %s; try again in a moment", ErrBusy, link.URL)
	}
}

func (s *Service) startSync(project int, url string) *flight {
	key := flightKey{project, url}
	s.mu.Lock()
	if f := s.flights[key]; f != nil {
		s.mu.Unlock()
		return f
	}
	f := &flight{done: make(chan struct{})}
	if s.flights == nil {
		s.flights = map[flightKey]*flight{}
	}
	s.flights[key] = f
	s.mu.Unlock()
	finish := func(source Source, err error) {
		f.source, f.err = source, err
		s.mu.Lock()
		delete(s.flights, key)
		s.mu.Unlock()
		close(f.done)
	}
	if !s.background(func(ctx context.Context) { finish(s.sync(ctx, project)) }) {
		finish(Source{}, fmt.Errorf("%w: the workspace is shutting down", ErrBusy))
	}
	return f
}

func (s *Service) syncing(project int, url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flights[flightKey{project, url}] != nil
}

// sync clones or fetches the project's current Official link, under the
// project's lock, and records the outcome.
func (s *Service) sync(ctx context.Context, project int) (Source, error) {
	unlock, err := s.lock(ctx, project)
	if err != nil {
		return Source{}, err
	}
	defer unlock()
	link, err := loadLink(ctx, s.DB, project)
	if err != nil {
		return Source{}, err
	}
	source, err := locate(ctx, s.DB, project, link)
	if err != nil || source.Mode != Official {
		return source, err
	}
	repo, _ := ParseGitHubURL(link.URL) // locate validated it
	source.Base, err = s.syncClone(ctx, source.Root, repo)
	if recordErr := recordSync(ctx, s.DB, project, link.URL, err); recordErr != nil && !errors.Is(recordErr, context.Canceled) {
		log.Printf("repolink: recording sync of project %d: %v", project, recordErr)
	}
	if err != nil {
		return Source{}, err
	}
	return source, nil
}

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
	exclude, err := os.OpenFile(filepath.Join(temp, ".git", "info", "exclude"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

// Status is a project's link plus, for an Official link, its clone.
type Status struct {
	Link
	Linked  bool `json:"linked"`
	Cloned  bool `json:"cloned"`
	Syncing bool `json:"syncing"`
	// FetchedAt is the last successful clone or fetch; Error the last failure,
	// cleared by the next success.
	FetchedAt *time.Time `json:"fetchedAt"`
	Error     string     `json:"error"`
}

func (s *Service) Status(ctx context.Context, project int) (Status, error) {
	link, err := loadLink(ctx, s.DB, project)
	if err != nil {
		return Status{}, err
	}
	status := Status{Link: link, Linked: link.Linked()}
	if link.Mode != Official || link.URL == "" {
		return status, nil
	}
	root, err := officialRoot(ctx, s.DB, project, link)
	if err != nil {
		status.Error = err.Error()
		return status, nil
	}
	if _, err = os.Stat(filepath.Join(root, ".git")); err == nil {
		status.Cloned = true
	}
	status.Syncing = s.syncing(project, link.URL)
	rec, err := loadSync(ctx, s.DB, project)
	if err != nil {
		return Status{}, err
	}
	if rec.url == link.URL {
		status.FetchedAt, status.Error = rec.fetchedAt, rec.err
	}
	return status, nil
}

// Patch changes a project's link; nil fields stay as they are.
type Patch struct {
	Mode *Mode   `json:"mode"`
	Path *string `json:"path"`
	URL  *string `json:"url"`
}

// Update validates and saves a link change. When the Official clone it points
// at changes (a new URL, or switching modes), the old clone is deleted in the
// background, agent branches included, and a new link starts cloning. It
// refuses while an agent is working in the old clone.
func (s *Service) Update(ctx context.Context, project int, patch Patch) (Status, error) {
	current, err := loadLink(ctx, s.DB, project)
	if err != nil {
		return Status{}, err
	}
	next := current
	if patch.Mode != nil {
		if *patch.Mode != Personal && *patch.Mode != Official {
			return Status{}, fmt.Errorf("%w: choose Personal or Official", ErrInvalid)
		}
		next.Mode = *patch.Mode
	}
	if patch.Path != nil {
		path := strings.TrimSpace(*patch.Path)
		if path != "" && (!filepath.IsAbs(path) || len(path) > 4096 || strings.ContainsRune(path, 0)) {
			return Status{}, fmt.Errorf("%w: use an absolute folder path, like /home/you/projects/app", ErrInvalid)
		}
		next.Path = path
	}
	if patch.URL != nil {
		next.URL = ""
		if raw := strings.TrimSpace(*patch.URL); raw != "" {
			repo, err := ParseGitHubURL(raw)
			if err != nil {
				return Status{}, err
			}
			next.URL = repo.URL()
		}
	}
	if next == current {
		return s.Status(ctx, project)
	}
	before, _ := officialRoot(ctx, s.DB, project, current)
	after, _ := officialRoot(ctx, s.DB, project, next)
	if before != "" && before != after {
		busy, err := agentWorkingIn(ctx, s.DB, before)
		if err != nil {
			return Status{}, err
		}
		if busy {
			return Status{}, fmt.Errorf("%w: an agent is working in Aycorn's copy of %s; wait for it to finish before changing the link", ErrBusy, current.URL)
		}
	}
	if err = saveLink(ctx, s.DB, project, next); err != nil {
		return Status{}, err
	}
	if before != after {
		s.background(func(ctx context.Context) { s.logPrune(ctx, project) })
		if after != "" {
			s.startSync(project, next.URL)
		}
	}
	return s.Status(ctx, project)
}

// Fetch clones or fetches an Official link now. It waits for the result until
// ctx ends; after that the status reports the work as still syncing.
func (s *Service) Fetch(ctx context.Context, project int) (Status, error) {
	link, err := loadLink(ctx, s.DB, project)
	if err != nil {
		return Status{}, err
	}
	if _, err = officialRoot(ctx, s.DB, project, link); err != nil {
		if errors.Is(err, ErrNotLinked) {
			return Status{}, fmt.Errorf("%w: only an Official GitHub link is fetched by Aycorn", ErrNotLinked)
		}
		return Status{}, err
	}
	f := s.startSync(project, link.URL)
	select {
	case <-f.done:
		if f.err != nil {
			return Status{}, f.err
		}
	case <-ctx.Done():
	}
	return s.Status(context.WithoutCancel(ctx), project)
}

// Forget removes deleted projects' clones in the background.
func (s *Service) Forget(projects ...int) {
	for _, project := range projects {
		s.background(func(ctx context.Context) { s.logPrune(ctx, project) })
	}
}

func (s *Service) logPrune(ctx context.Context, project int) {
	if err := s.prune(ctx, project); err != nil && ctx.Err() == nil {
		log.Printf("repolink: removing old clones of project %d: %v", project, err)
	}
}

// prune removes everything under the project's clone directory except the
// clone its current Official link uses: an old link's clone, a deleted
// project's, or an interrupted clone.
func (s *Service) prune(ctx context.Context, project int) error {
	unlock, err := s.lock(ctx, project)
	if err != nil {
		return err
	}
	defer unlock()
	repos, err := reposDir(ctx, s.DB)
	if err != nil {
		return err
	}
	dir := projectDir(repos, project)
	keep := ""
	link, err := loadLink(ctx, s.DB, project)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		keep, _ = officialRoot(ctx, s.DB, project, link)
	}
	if keep == "" {
		return os.RemoveAll(dir)
	}
	owners, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, owner := range owners {
		ownerDir := filepath.Join(dir, owner.Name())
		if ownerDir != filepath.Dir(keep) {
			errs = append(errs, os.RemoveAll(ownerDir))
			continue
		}
		clones, err := os.ReadDir(ownerDir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, clone := range clones {
			if path := filepath.Join(ownerDir, clone.Name()); path != keep {
				errs = append(errs, os.RemoveAll(path))
			}
		}
	}
	return errors.Join(errs...)
}

// pruneAll prunes every project directory found at startup.
func (s *Service) pruneAll(ctx context.Context) {
	repos, err := reposDir(ctx, s.DB)
	if err != nil {
		log.Printf("repolink: %v", err)
		return
	}
	entries, err := os.ReadDir(repos)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("repolink: %v", err)
		}
		return
	}
	for _, entry := range entries {
		if project, err := strconv.Atoi(entry.Name()); err == nil && entry.IsDir() {
			s.logPrune(ctx, project)
		}
	}
}
