package environments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

type Service struct {
	Store      *Store
	Runtime    Runtime
	Root       string
	mu         sync.Mutex
	operations map[int]operation
	builds     chan struct{}
	wg         sync.WaitGroup
}
type operation struct {
	desired string
	cancel  context.CancelFunc
}

type SourceStatus struct {
	Changed bool   `json:"changed"`
	Commit  string `json:"commit"`
	Message string `json:"message"`
}

func (s *Service) SourceStatus(ctx context.Context, id int) (SourceStatus, error) {
	e, err := s.Store.Get(id)
	if err != nil {
		return SourceStatus{}, err
	}
	commit, err := worktree.ResolveBranch(ctx, e.Repo, e.Branch)
	if err != nil {
		return SourceStatus{}, err
	}
	result := SourceStatus{Commit: commit, Changed: commit != e.Commit, Message: "This preview matches the current source."}
	if !result.Changed && e.IncludeChanges && e.Digest != "" {
		digest, err := worktree.CurrentWorkspaceDigest(ctx, e.Repo, e.Branch)
		if err != nil {
			return SourceStatus{}, err
		}
		result.Changed = digest != e.Digest
	}
	if result.Changed {
		result.Message = "Newer source changes are available. Rebuild to preview them."
	}
	return result, nil
}

func (s *Service) Branches(ctx context.Context, project int) ([]string, error) {
	var root string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT COALESCE(repoPath,'') FROM project WHERE id=?`, project).Scan(&root); err != nil {
		return nil, err
	}
	if root == "" {
		return []string{}, nil
	}
	return worktree.LocalBranches(ctx, root)
}

func (s *Service) BranchSources(ctx context.Context, project int) ([]worktree.BranchSource, error) {
	var root string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT COALESCE(repoPath,'') FROM project WHERE id=?`, project).Scan(&root); err != nil {
		return nil, err
	}
	if root == "" {
		return []worktree.BranchSource{}, nil
	}
	return worktree.LocalBranchSources(ctx, root)
}

func (s *Service) requireInactiveBranch(e *Environment) error {
	active, err := (&repos.AgentJobRepo{DB: s.Store.DB}).BranchHasActiveRun(e.Repo, e.Branch)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: an agent is still changing this branch; wait for it to finish or choose Latest commit", ErrConflict)
	}
	return nil
}

func (s *Service) Create(ctx context.Context, project int, input CreateInput) (*Environment, error) {
	if project <= 0 {
		return nil, ErrInvalid
	}
	if input.RequestKey == "" {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return nil, err
		}
		input.RequestKey = hex.EncodeToString(bytes[:])
	}
	if len(input.RequestKey) > 128 || strings.ContainsRune(input.RequestKey, 0) {
		return nil, ErrInvalid
	}
	if existing, err := s.Store.Existing(project, input.RequestKey); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	settings, err := s.Store.Settings(project)
	if err != nil {
		return nil, err
	}
	if err = settings.Validate(true); err != nil {
		return nil, err
	}
	e := &Environment{ProjectID: project, TaskID: input.TaskID, JobID: input.JobID, RequestKey: input.RequestKey, Settings: settings, Branch: input.Branch, IncludeChanges: input.IncludeChanges}
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT COALESCE(repoPath,'') FROM project WHERE id=?`, project).Scan(&e.Repo); err != nil {
		return nil, err
	}
	if input.JobID > 0 {
		var state, repo, branch string
		var task int
		err = s.Store.DB.QueryRowContext(ctx, `SELECT j.task,j.status,COALESCE(json_extract(j.requestJson,'$.repoPath'),''),COALESCE(json_extract(a.artifactJson,'$.branch'),'') FROM agent_job j JOIN agent_run a ON a.job=j.id JOIN task t ON t.id=j.task JOIN checklist c ON c.id=t.checklist WHERE j.id=? AND c.project=? ORDER BY a.id DESC LIMIT 1`, input.JobID, project).Scan(&task, &state, &repo, &branch)
		if err != nil {
			return nil, err
		}
		if input.TaskID != 0 && task != input.TaskID {
			return nil, fmt.Errorf("%w: this run belongs to a different task", ErrInvalid)
		}
		if state != "completed" && state != "failed" && state != "canceled" && state != "interrupted" {
			return nil, fmt.Errorf("%w: wait for the agent run to finish before capturing its files", ErrConflict)
		}
		if repo != e.Repo {
			return nil, fmt.Errorf("%w: the project's repository has changed since this run", ErrConflict)
		}
		e.Branch = branch
		e.TaskID = task
		e.IncludeChanges = true
	} else if input.TaskID != 0 {
		return nil, fmt.Errorf("%w: select a run when creating a task preview", ErrInvalid)
	}
	if e.Repo == "" || e.Branch == "" {
		return nil, fmt.Errorf("%w: link a repository and select a branch first", ErrInvalid)
	}
	e.Commit, err = worktree.ResolveBranch(ctx, e.Repo, e.Branch)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if e.IncludeChanges {
		state, err := worktree.InspectBranch(ctx, e.Repo, e.Branch, e.Commit)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read the branch's working tree: %v", ErrInvalid, err)
		}
		if state.Workspace == "" {
			return nil, fmt.Errorf("%w: this branch has no working tree; choose Latest commit or check out the branch first", ErrInvalid)
		}
		if err = s.requireInactiveBranch(e); err != nil {
			return nil, err
		}
	}
	e.Name = e.Branch
	if len(e.Name) > 120 {
		e.Name = e.Name[:120]
	}
	return s.Store.Insert(e)
}

func (s *Service) Rebuild(ctx context.Context, id int, input CreateInput) (*Environment, error) {
	e, err := s.Store.Get(id)
	if err != nil {
		return nil, err
	}
	input.Branch = e.Branch
	input.TaskID = e.TaskID
	input.JobID = e.JobID
	input.IncludeChanges = e.IncludeChanges
	return s.Create(ctx, e.ProjectID, input)
}

func (s *Service) Start(ctx context.Context) error {
	if !filepath.IsAbs(s.Root) {
		return fmt.Errorf("environment storage must be an absolute path")
	}
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return err
	}
	s.operations = map[int]operation{}
	s.builds = make(chan struct{}, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := s.tick(ctx); err != nil {
				log.Printf("environments: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
func (s *Service) Wait() { s.wg.Wait(); s.Runtime.Close() }

func (s *Service) tick(ctx context.Context) error {
	if err := s.Store.Expire(ctx); err != nil {
		return err
	}
	if err := s.autoPreview(ctx); err != nil {
		return err
	}
	environments, err := s.Store.List(0, 0, true)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range environments {
		if active, ok := s.operations[e.ID]; ok {
			if active.desired != e.Desired {
				active.cancel()
			}
			continue
		}
		if e.State == "failed" && e.Desired == "running" {
			continue
		}
		operationCtx, cancel := context.WithCancel(ctx)
		s.operations[e.ID] = operation{desired: e.Desired, cancel: cancel}
		s.wg.Add(1)
		go func(e Environment) {
			defer s.wg.Done()
			defer cancel()
			defer func() { s.mu.Lock(); delete(s.operations, e.ID); s.mu.Unlock() }()
			s.reconcile(operationCtx, &e)
		}(e)
	}
	return nil
}

func (s *Service) reconcile(ctx context.Context, e *Environment) {
	logLine := func(message string) { s.Store.Log(e.ID, message) }
	observe := func(state string) { e.State = state; _ = s.Store.Observe(e) }
	if e.Desired != "running" {
		bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		var err error
		if e.Desired == "deleted" {
			observe("deleting")
			err = s.Runtime.Destroy(bounded, e)
		} else {
			observe("stopping")
			err = s.Runtime.Stop(bounded, e)
		}
		if err != nil {
			if ctx.Err() == nil {
				e.Error = err.Error()
				_ = s.Store.Observe(e)
			}
			return
		}
		e.URL = ""
		e.Error = ""
		if e.Desired == "deleted" {
			if err = os.RemoveAll(s.directory(e.ID)); err != nil {
				e.Error = err.Error()
				_ = s.Store.Observe(e)
				return
			}
			observe("deleted")
		} else {
			observe("stopped")
		}
		return
	}
	if e.State == "ready" || e.State == "unavailable" {
		bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		observation, err := s.Runtime.Inspect(bounded, e)
		if ctx.Err() != nil {
			return
		}
		if err != nil || !observation.Ready {
			e.URL = ""
			e.Error = observation.Message
			if err != nil {
				e.Error = err.Error()
			}
			if e.Error == "" {
				e.Error = "Preview is not ready; checking again shortly."
			}
			// A failed readiness check does not prove the workload has stopped.
			// Keep polling, without rebuilding or rerunning the test Job.
			observe("unavailable")
		} else {
			e.URL = observation.URL
			e.Error = ""
			observe("ready")
		}
		return
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(e.Settings.TimeoutSeconds)*time.Second)
	defer cancel()
	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		e.Error = err.Error()
		e.URL = ""
		observe("failed")
		logLine("\n" + err.Error() + "\n")
	}
	if e.Digest == "" {
		observe("snapshotting")
		if err := s.snapshot(bounded, e, logLine); err != nil {
			fail(err)
			return
		}
		if err := s.Store.Observe(e); err != nil {
			fail(err)
			return
		}
	}
	if e.Image == "" {
		observe("building")
		select {
		case s.builds <- struct{}{}:
		case <-bounded.Done():
			fail(bounded.Err())
			return
		}
		image, testImage, err := s.Runtime.Build(bounded, e, s.directory(e.ID), logLine)
		<-s.builds
		if err != nil {
			fail(err)
			return
		}
		e.Image = image
		e.TestImage = testImage
		if err = s.Store.Observe(e); err != nil {
			fail(err)
			return
		}
	}
	observe("starting")
	if e.TestImage != "" {
		e.TestState = "running"
		_ = s.Store.Observe(e)
	}
	if err := s.Runtime.Start(bounded, e, logLine); err != nil {
		fail(err)
		return
	}
	for {
		observation, err := s.Runtime.Inspect(bounded, e)
		if err == nil && observation.Ready {
			e.URL = observation.URL
			e.Error = ""
			observe("ready")
			logLine("\nPreview ready at " + e.URL + "\n")
			return
		}
		e.Error = observation.Message
		if err != nil {
			e.Error = err.Error()
		}
		_ = s.Store.Observe(e)
		select {
		case <-bounded.Done():
			fail(fmt.Errorf("preview did not become ready before timeout: %w", bounded.Err()))
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *Service) directory(id int) string {
	return filepath.Join(s.Root, fmt.Sprintf("environment-%d", id))
}
func (s *Service) snapshot(ctx context.Context, e *Environment, logLine func(string)) error {
	directory := s.directory(e.ID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	metadata := filepath.Join(directory, "snapshot.json")
	source := filepath.Join(directory, "source")
	if raw, err := os.ReadFile(metadata); err == nil {
		var snap worktree.SourceSnapshot
		if json.Unmarshal(raw, &snap) != nil || snap.Commit != e.Commit || len(snap.Digest) != 64 {
			return fmt.Errorf("stored snapshot metadata is invalid; create a fresh preview")
		}
		if _, err = os.Stat(source); err != nil {
			return fmt.Errorf("stored source is missing; create a fresh preview")
		}
		e.Digest = snap.Digest
		return nil
	}
	if e.JobID > 0 {
		var active bool
		if err := s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_job WHERE id=? AND status IN ('pending','claimed','running','canceling'))`, e.JobID).Scan(&active); err != nil {
			return err
		}
		if active {
			return fmt.Errorf("agent restarted before source capture; stop this preview and wait for the run")
		}
	}
	if e.IncludeChanges {
		if err := s.requireInactiveBranch(e); err != nil {
			return err
		}
	}
	temp, err := os.MkdirTemp(directory, "capturing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	snap, err := worktree.SnapshotSource(ctx, e.Repo, e.Branch, e.Commit, e.IncludeChanges, temp)
	if err != nil {
		return err
	}
	if e.IncludeChanges {
		if err = s.requireInactiveBranch(e); err != nil {
			return err
		}
	}
	if e.Settings.Profile == "aycorn" {
		marker, err := os.ReadFile(filepath.Join(temp, "server/cmd/web/preview.go"))
		if err != nil || !strings.Contains(string(marker), "PreviewProtocolVersion = 1") {
			if !e.IncludeChanges {
				return fmt.Errorf("the selected commit does not include Aycorn's preview support; choose Working tree in Environments to include uncommitted changes, or select a branch with the support committed")
			}
			return fmt.Errorf("this working tree does not include Aycorn's preview support; use a working tree containing the Kubernetes environment implementation")
		}
	}
	if err = os.RemoveAll(source); err != nil {
		return err
	}
	if err = os.Rename(temp, source); err != nil {
		return err
	}
	raw, _ := json.Marshal(snap)
	if err = os.WriteFile(metadata, raw, 0600); err != nil {
		return err
	}
	e.Digest = snap.Digest
	logLine(fmt.Sprintf("Captured %s at %s\nSnapshot SHA-256: %s\n", e.Branch, e.Commit, e.Digest))
	if len(snap.Excluded) > 0 {
		logLine("Excluded data, dependencies, or credentials: " + strings.Join(snap.Excluded, ", ") + "\n")
	}
	return nil
}

// Auto previews are independent of Conductor's review handoff. They never change
// a task, rerun an agent, or retry a failed preview. The request key deduplicates
// restarts and polling; enabling the setting only applies to subsequent finishes.
func (s *Service) autoPreview(ctx context.Context) error {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT c.project,t.id,j.id FROM conductor_task ct JOIN task t ON t.id=ct.task JOIN checklist c ON c.id=t.checklist JOIN agent_job j ON j.id=ct.job JOIN environment_settings es ON es.project=c.project WHERE ct.state='completed' AND j.status='completed' AND json_extract(es.settings,'$.autoPreview')=1 AND unixepoch(j.finishedAt)>=es.enabledAt AND NOT EXISTS(SELECT 1 FROM task_environment e WHERE e.project=c.project AND e.requestKey='conductor-'||j.id) ORDER BY j.id LIMIT 20`)
	if err != nil {
		return err
	}
	type candidate struct{ project, task, job int }
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.project, &c.task, &c.job); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		_, _ = s.Create(ctx, c.project, CreateInput{TaskID: c.task, JobID: c.job, RequestKey: fmt.Sprintf("conductor-%d", c.job)})
	}
	return nil
}
