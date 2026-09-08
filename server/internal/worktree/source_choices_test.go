package worktree

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLocalBranchSourcesDistinguishesCurrentAndOtherWorktrees(t *testing.T) {
	root := snapshotFixture(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "other working tree")
	for _, args := range [][]string{{"branch", "committed-only"}, {"worktree", "add", "-b", "feature", other}} {
		if _, err := gitOutput(ctx, root, args...); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := LocalBranchSources(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []BranchSource{
		{Name: "committed-only"},
		{Name: "feature", HasWorktree: true},
		{Name: "main", Current: true, HasWorktree: true},
	}
	if !reflect.DeepEqual(sources, want) {
		t.Fatalf("branch sources = %#v, want %#v", sources, want)
	}
	branches, err := LocalBranches(ctx, root)
	if err != nil || !reflect.DeepEqual(branches, []string{"committed-only", "feature", "main"}) {
		t.Fatalf("plain branch names changed: %#v, %v", branches, err)
	}
	if _, err = gitOutput(ctx, root, "checkout", "--detach"); err != nil {
		t.Fatal(err)
	}
	sources, err = LocalBranchSources(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.Current || (source.Name == "main" && source.HasWorktree) {
			t.Fatalf("detached HEAD was attributed to a branch: %#v", source)
		}
	}
}
