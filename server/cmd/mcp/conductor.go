package main

import (
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"strings"
)

type StartConductorTaskInput struct {
	ProjectID int    `json:"projectId"`
	TaskID    int    `json:"taskId"`
	Role      string `json:"role" jsonschema:"coder, researcher, reviewer, or planner"`
}
type DeferConductorTaskInput struct {
	ProjectID int    `json:"projectId"`
	TaskID    int    `json:"taskId"`
	Reason    string `json:"reason"`
}

type StartedConductorTask struct {
	JobID  int    `json:"jobId"`
	TaskID int    `json:"taskId"`
	Status string `json:"status"`
}

func (t *toolset) listConductorTasks(ctx context.Context, _ *mcp.CallToolRequest, _ NoInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := t.checkProjectReadScope(); err != nil {
		return nil, nil, err
	}
	if t.runDispatchID <= 0 || t.conductorService == nil {
		return nil, nil, fmt.Errorf("requires a dispatcher session")
	}
	tasks, err := t.conductorService.Candidates(ctx, t.runProjectID)
	return nil, map[string]any{"tasks": tasks}, err
}
func (t *toolset) startConductorTask(ctx context.Context, _ *mcp.CallToolRequest, in StartConductorTaskInput) (*mcp.CallToolResult, *StartedConductorTask, error) {
	if t.runDispatchID <= 0 || in.ProjectID != t.runProjectID || t.conductorService == nil {
		return nil, nil, fmt.Errorf("task start is outside dispatcher scope")
	}
	job, err := t.conductorService.StartTask(ctx, in.ProjectID, in.TaskID, t.runDispatchID, in.Role)
	if err != nil {
		return nil, nil, err
	}
	return nil, &StartedConductorTask{JobID: job.ID, TaskID: job.Task, Status: job.Status}, nil
}
func (t *toolset) deferConductorTask(_ context.Context, _ *mcp.CallToolRequest, in DeferConductorTaskInput) (*mcp.CallToolResult, map[string]bool, error) {
	if t.runDispatchID <= 0 || in.ProjectID != t.runProjectID || t.conductorService == nil {
		return nil, nil, fmt.Errorf("task is outside dispatcher scope")
	}
	if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 4000 {
		return nil, nil, fmt.Errorf("provide a specific blocker under 4000 characters")
	}
	err := t.conductorService.Repo.DeferTask(in.ProjectID, in.TaskID, t.runDispatchID, in.Reason)
	return nil, map[string]bool{"deferred": err == nil}, err
}
