package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
	"os"
	"time"
)

func (w *Worker) runExplicit(parent context.Context, job *models.AgentJob) (bool, error) {
	repo := w.JobService.JobRepo
	ok, err := repo.MarkRunning(job.ID)
	if err != nil {
		return true, err
	}
	if !ok {
		return true, nil
	}
	if err = repo.BeginAttempt(job.ID); err != nil {
		_, _ = repo.Fail(job.ID, err.Error())
		return true, err
	}
	artifacts := models.AIRunArtifacts{}
	result := harness.RunResult{UsageJson: "{}"}
	// Every normal exit retains output/artifacts, including setup failures.
	finish := func(runErr error) (bool, error) {
		status := "completed"
		message := ""
		if runErr != nil {
			status = "failed"
			message = runErr.Error()
			result.ExitCode = 1
		}
		current, getErr := repo.FindOne(job.ID)
		if getErr != nil {
			return true, getErr
		}
		if current.Status == "canceling" {
			status = "canceled"
			message = "Stopped by you"
		} else if parent.Err() != nil {
			status = "interrupted"
			message = "Server stopped during the run"
		}
		if err := repo.FinishAI(job.ID, status, message, result.Output, result.UsageJson, result.ExitCode, artifacts); err != nil {
			return true, err
		}
		return true, nil
	}
	if job.Request == nil {
		return finish(errors.New("Start a new explicit run; this job has no request snapshot"))
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatch:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := repo.FindOne(job.ID)
				if err != nil || current.Status == "canceling" {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { close(stopWatch); <-watchDone }()
	spec := harness.RunSpec{JobID: job.ID, TaskID: job.Task, PersonaID: job.Persona, Request: job.Request}
	var wt *worktree.Worktree
	if job.Request.RepoPath != "" {
		wt, artifacts.BaseCommit, err = worktree.CreateRun(ctx, job.Request.RepoPath, job.Request.Key)
		if err != nil {
			return finish(fmt.Errorf("create workspace: %w", err))
		}
		artifacts.Workspace = wt.Path
		artifacts.Branch = wt.Branch
		spec.WorkDir = wt.Path
	} else {
		spec.WorkDir, err = os.MkdirTemp("", "aycorn-ask-")
		if err != nil {
			return finish(err)
		}
		defer os.RemoveAll(spec.WorkDir)
	}
	if err = repo.Checkpoint(job.ID, "", "{}", artifacts); err != nil {
		return finish(err)
	}
	if err = repo.SetProgress(job.ID, "Starting OpenCode"); err != nil {
		return finish(err)
	}
	spec.OnProgress = func(progress, output string) error {
		if err := repo.Checkpoint(job.ID, output, "{}", artifacts); err != nil {
			return err
		}
		return repo.SetProgress(job.ID, progress)
	}
	result, err = w.Harness.Run(ctx, spec)
	// Use a separate bounded context after cancellation so partial edits survive.
	if wt != nil {
		captureCtx, stop := context.WithTimeout(context.Background(), 20*time.Second)
		diff, files, captureErr := wt.Capture(captureCtx, artifacts.BaseCommit)
		stop()
		artifacts.Diff = diff
		artifacts.Files = files
		if captureErr != nil {
			artifacts.Warning = captureErr.Error()
			if err == nil {
				err = fmt.Errorf("work finished but changes could not be captured: %w", captureErr)
			}
		}
	}
	return finish(err)
}
