package repolink

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
