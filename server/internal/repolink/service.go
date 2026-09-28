package repolink

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"

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
