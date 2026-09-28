package environments

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/repolink"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

// syncedSource stands in for repolink.Service: the clone is already made.
type syncedSource struct {
	source repolink.Source
	syncs  int
}

func (s *syncedSource) SourceRepo(context.Context, int) (repolink.Source, error) {
	s.syncs++
	return s.source, nil
}

func TestOfficialPreviewsBuildFromRemoteBranches(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	upstream := repoFixture(t, map[string]string{"app.txt": "first"})
	sourceGit(t, upstream, "branch", "feature")
	configureFixture(t, store, upstream)
	if _, err := store.DB.Exec(`UPDATE project SET repoMode='official', repoUrl='https://github.com/acme/app' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	located, err := repolink.Locate(ctx, store.DB, 1)
	if err != nil {
		t.Fatal(err)
	}
	sourceGit(t, upstream, "clone", "-q", upstream, located.Root)
	sources := &syncedSource{source: repolink.Source{Mode: repolink.Official, Root: located.Root}}
	s := &Service{Store: store, Root: t.TempDir(), Sources: sources}

	branches, err := s.BranchSources(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []worktree.BranchSource{{Name: "origin/feature", Remote: true}, {Name: "origin/main", Current: true, Remote: true}}
	if !reflect.DeepEqual(branches, want) || sources.syncs != 1 {
		t.Fatalf("branches = %+v (synced %d times); want the remote branches with main as default", branches, sources.syncs)
	}
	for _, input := range []CreateInput{{Branch: "origin/main", IncludeChanges: true}, {Branch: "main"}, {Branch: "origin/HEAD"}} {
		if _, err = s.Create(ctx, 1, input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", input, err)
		}
	}
	first := sourceGit(t, upstream, "rev-parse", "main")
	e, err := s.Create(ctx, 1, CreateInput{Branch: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	if stored, _ := store.Get(e.ID); !stored.Remote || stored.Commit != first || stored.Repo != located.Root || stored.IncludeChanges {
		t.Fatalf("preview source = %+v", stored)
	}

	// The upstream moves on; the preview notices once the clone has fetched.
	writeProjectSource(t, upstream, "app.txt", "second")
	sourceGit(t, upstream, "commit", "-q", "-am", "second")
	if status, err := s.SourceStatus(ctx, e.ID); err != nil || status.Changed {
		t.Fatalf("before fetching: %+v, %v", status, err)
	}
	sourceGit(t, located.Root, "fetch", "-q", "origin")
	if status, err := s.SourceStatus(ctx, e.ID); err != nil || !status.Changed {
		t.Fatalf("after fetching: %+v, %v", status, err)
	}

	// The build still uses the commit the preview was created at.
	if err = s.snapshot(ctx, e, func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.directory(e.ID), "source", "app.txt"))
	if err != nil || string(got) != "first" || len(e.Digest) != 64 {
		t.Fatalf("snapshot = %q, %v (digest %q)", got, err, e.Digest)
	}
}
