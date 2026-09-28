package repolink

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

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
