package worker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/repolink"
)

type fakeSources struct {
	source  repolink.Source
	err     error
	project int
}

func (f *fakeSources) SourceRepo(_ context.Context, project int) (repolink.Source, error) {
	f.project = project
	return f.source, f.err
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.test"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repoWithTwoCommits stands in for an Official clone whose checked-out branch
// is newer than the remote default branch it last fetched.
func repoWithTwoCommits(t *testing.T) (root, first, second string) {
	t.Helper()
	root = t.TempDir()
	gitOut(t, root, "init", "-q")
	gitOut(t, root, "commit", "-q", "--allow-empty", "-m", "first")
	first = gitOut(t, root, "rev-parse", "HEAD")
	gitOut(t, root, "commit", "-q", "--allow-empty", "-m", "second")
	return root, first, gitOut(t, root, "rev-parse", "HEAD")
}

func enqueueRepositoryRun(t *testing.T, s *services.AgentJobService, root string) *models.AgentJob {
	t.Helper()
	j, err := s.JobRepo.EnqueueAI(1, 0, models.AIRunRequest{Intent: "implement", Model: "test/model", RepoPath: root, ProjectID: 1, Key: strings.Repeat("c", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestRunsSyncTheRepositoryAndStartFromItsBase(t *testing.T) {
	s := testService(t)
	root, first, _ := repoWithTwoCommits(t)
	j := enqueueRepositoryRun(t, s, root)
	var head string
	h := &testEngine{run: func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		head = gitOut(t, spec.WorkDir, "rev-parse", "HEAD")
		return harness.RunResult{Output: "Done", UsageJson: "{}"}, nil
	}}
	w := New(s, h)
	sources := &fakeSources{source: repolink.Source{Mode: repolink.Official, Root: root, Base: first}}
	w.Sources = sources
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := s.ListByTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if sources.project != 1 || data.Jobs[0].ID != j.ID || data.Jobs[0].Status != "completed" {
		t.Fatalf("run did not sync its project's repository: %+v", data.Jobs[0])
	}
	if head != first || data.Runs[0].Artifacts.BaseCommit != first {
		t.Fatalf("run started at %s (recorded %s); want the fetched default branch %s", head, data.Runs[0].Artifacts.BaseCommit, first)
	}
}

func TestRunsStopWhenTheRepositoryCantBeUsed(t *testing.T) {
	for name, sources := range map[string]*fakeSources{
		"link changed": {source: repolink.Source{Mode: repolink.Official, Root: "/elsewhere"}},
		"sync failed":  {err: fmt.Errorf("%w: the server's git login can't read acme/app", repolink.ErrSync)},
	} {
		t.Run(name, func(t *testing.T) {
			s := testService(t)
			root, _, _ := repoWithTwoCommits(t)
			enqueueRepositoryRun(t, s, root)
			h := &testEngine{}
			w := New(s, h)
			w.Sources = sources
			if _, err := w.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			data, err := s.ListByTask(1)
			if err != nil {
				t.Fatal(err)
			}
			job := data.Jobs[0]
			if job.Status != "failed" || h.calls.Load() != 0 {
				t.Fatalf("run went ahead: %+v (harness calls %d)", job, h.calls.Load())
			}
			if want := map[string]string{"link changed": "start it again", "sync failed": "can't read acme/app"}[name]; !strings.Contains(job.Error, want) {
				t.Fatalf("error %q; want it to mention %q", job.Error, want)
			}
		})
	}
}
