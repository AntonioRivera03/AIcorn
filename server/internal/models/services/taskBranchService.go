package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

func (s *AIService) TaskBranches(ctx context.Context, taskID int) ([]models.TaskBranch, error) {
	if _, err := s.Tasks.FindOne(int64(taskID)); err != nil {
		return nil, err
	}
	branches, err := s.Jobs.TaskBranches(taskID)
	if err != nil {
		return nil, err
	}
	for i := range branches {
		b := &branches[i]
		active, err := s.Jobs.BranchHasActiveRun(b.RepoPath, b.Branch)
		if err != nil {
			return nil, err
		}
		if active {
			b.Status = "running"
		}
		state, err := worktree.InspectBranch(ctx, b.RepoPath, b.Branch, b.BaseCommit)
		if err != nil {
			b.Problem = err.Error()
			continue
		}
		b.Workspace, b.Dirty, b.MergedInto = state.Workspace, state.Dirty, state.MergedInto
	}
	return branches, nil
}

func (s *AIService) taskBranch(taskID, jobID int) (*models.TaskBranch, error) {
	branches, err := s.Jobs.TaskBranches(taskID)
	if err != nil {
		return nil, err
	}
	for _, branch := range branches {
		if branch.JobID == jobID {
			switch branch.Status {
			case "completed", "failed", "canceled", "interrupted":
				active, err := s.Jobs.BranchHasActiveRun(branch.RepoPath, branch.Branch)
				if err != nil {
					return nil, err
				}
				if active {
					return nil, fmt.Errorf("%w: wait for the active chat turn to finish", worktree.ErrMergeBlocked)
				}
				return &branch, nil
			default:
				return nil, fmt.Errorf("%w: wait for the agent run to finish before merging", worktree.ErrMergeBlocked)
			}
		}
	}
	return nil, sql.ErrNoRows
}
func (s *AIService) PreviewTaskMerge(ctx context.Context, taskID, jobID int, target string) (*worktree.MergePreview, error) {
	b, err := s.taskBranch(taskID, jobID)
	if err != nil {
		return nil, err
	}
	preview, err := worktree.PreviewMerge(ctx, b.RepoPath, b.Branch, target)
	if err != nil {
		return nil, err
	}
	active, err := s.Jobs.BranchHasActiveRun(b.RepoPath, preview.Target)
	if err != nil {
		return nil, err
	}
	if active {
		preview.Blocked = "An agent is working on the destination branch. Wait for that run to finish."
	}
	return preview, nil
}
func (s *AIService) MergeTaskBranch(ctx context.Context, taskID, jobID int, target, token string) (*worktree.MergeResult, error) {
	b, err := s.taskBranch(taskID, jobID)
	if err != nil {
		return nil, err
	}
	active, err := s.Jobs.BranchHasActiveRun(b.RepoPath, target)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("%w: an agent is working on the destination branch", worktree.ErrMergeBlocked)
	}
	return worktree.MergeBranch(ctx, b.RepoPath, b.Branch, target, token, taskID, jobID)
}
