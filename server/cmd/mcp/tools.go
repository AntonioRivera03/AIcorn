package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func parseRFC3339(s string) (*time.Time, error) {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

// toolset holds the services each MCP tool needs and registers them on a
// server. One method per tool, one atomic operation per tool — no combined
// "manage_task" tool that branches on an action field, since that pattern
// causes agents to sequence calls incorrectly.
type toolset struct {
	taskService    *services.TaskService
	projectService *services.ProjectService
	stageService   *services.StageService
}

func (t *toolset) register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_tasks",
		Description: "Search and filter tasks across all projects. Call list_workflow_stages first if filtering by stage.",
	}, t.searchTasks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_task",
		Description: "Read a single task's full details by id.",
	}, t.readTask)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects.",
	}, t.listProjects)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_workflow_stages",
		Description: "List all workflow stages. Use this to learn valid stage ids before calling search_tasks, create_task, or move_task_stage.",
	}, t.listWorkflowStages)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a new task on a checklist. The task body is left empty — write it through update_task or the app after creation.",
	}, t.createTask)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_task",
		Description: "Update a task's name, priority, or assignee. Never changes stage — use move_task_stage for that.",
	}, t.updateTask)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "move_task_stage",
		Description: "Move a task to a different workflow stage. Requires the stage you currently believe the task is in (fromStage); fails safely if the task has moved since you last read it.",
	}, t.moveTaskStage)
}

type SearchTasksInput struct {
	Query      string   `json:"query,omitempty" jsonschema:"free-text search over the task name"`
	ProjectIDs []int    `json:"projectIds,omitempty"`
	StageIDs   []int    `json:"stageIds,omitempty" jsonschema:"stage ids — call list_workflow_stages for valid ids"`
	Priorities []string `json:"priorities,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	Assignees  []string `json:"assignees,omitempty"`
	Limit      int      `json:"limit,omitempty" jsonschema:"default 25, max 100"`
}

func (t *toolset) searchTasks(ctx context.Context, req *mcp.CallToolRequest, in SearchTasksInput) (*mcp.CallToolResult, []models.TaskWithProject, error) {
	filters := &repos.TaskFilters{
		SearchQuery:    in.Query,
		ProjectIDQuery: in.ProjectIDs,
		PriorityQuery:  in.Priorities,
		AssigneeQuery:  in.Assignees,
	}
	for _, id := range in.StageIDs {
		filters.StageQuery = append(filters.StageQuery, strconv.Itoa(id))
	}

	tasks, err := t.taskService.GetAllTasks(filters)
	if err != nil {
		return nil, nil, err
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return nil, tasks, nil
}

type ReadTaskInput struct {
	TaskID int `json:"taskId"`
}

func (t *toolset) readTask(ctx context.Context, req *mcp.CallToolRequest, in ReadTaskInput) (*mcp.CallToolResult, *models.TaskWithProject, error) {
	task, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, nil, err
	}
	return nil, task, nil
}

type NoInput struct{}

func (t *toolset) listProjects(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, []models.Project, error) {
	projects, err := t.projectService.GetAllProjects()
	if err != nil {
		return nil, nil, err
	}
	return nil, projects, nil
}

func (t *toolset) listWorkflowStages(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, []models.Stage, error) {
	stages, err := t.stageService.GetAllStages()
	if err != nil {
		return nil, nil, err
	}
	return nil, stages, nil
}

type CreateTaskInput struct {
	ChecklistID  int    `json:"checklistId"`
	StageID      int    `json:"stageId" jsonschema:"call list_workflow_stages for valid ids — typically the workflow's open stage for a new task"`
	Name         string `json:"name"`
	Priority     string `json:"priority,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	TypeID       int    `json:"typeId,omitempty" jsonschema:"omit to use the project's default task type"`
	Assignee     string `json:"assignee,omitempty"`
	PlannedStart string `json:"plannedStart,omitempty" jsonschema:"RFC3339 timestamp"`
	PlannedEnd   string `json:"plannedEnd,omitempty" jsonschema:"RFC3339 timestamp"`
}

func (t *toolset) createTask(ctx context.Context, req *mcp.CallToolRequest, in CreateTaskInput) (*mcp.CallToolResult, *models.ChecklistTask, error) {
	task := &models.ChecklistTask{
		Task: models.Task{
			Checklist: in.ChecklistID,
			Stage:     in.StageID,
			Name:      in.Name,
			Priority:  in.Priority,
			Assignee:  in.Assignee,
			Type:      models.TaskType{ID: in.TypeID},
		},
	}

	if in.PlannedStart != "" {
		ts, err := parseRFC3339(in.PlannedStart)
		if err != nil {
			return nil, nil, err
		}
		task.TimePlannedStart = ts
		task.HasTimePlannedStart = true
	}
	if in.PlannedEnd != "" {
		ts, err := parseRFC3339(in.PlannedEnd)
		if err != nil {
			return nil, nil, err
		}
		task.TimePlannedEnd = ts
		task.HasTimePlannedEnd = true
	}

	newTask, err := t.taskService.CreateChecklistTask(task)
	if err != nil {
		return nil, nil, err
	}
	return nil, newTask, nil
}

// UpdateTaskInput is deliberately narrower than PUT /api/task's full
// ChecklistTask body. TaskService.UpdateTask writes every column in one shot
// including stage; if this tool round-tripped a caller-supplied stage
// verbatim, an agent working from stale context could silently move a task
// sideways of move_task_stage's CAS check.
type UpdateTaskInput struct {
	TaskID   int     `json:"taskId"`
	Name     *string `json:"name,omitempty"`
	Priority *string `json:"priority,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	Assignee *string `json:"assignee,omitempty"`
}

func (t *toolset) updateTask(ctx context.Context, req *mcp.CallToolRequest, in UpdateTaskInput) (*mcp.CallToolResult, bool, error) {
	current, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, false, err
	}
	if in.Name != nil {
		current.Name = *in.Name
	}
	if in.Priority != nil {
		current.Priority = *in.Priority
	}
	if in.Assignee != nil {
		current.Assignee = *in.Assignee
	}
	// current.Stage is whatever it already was — never set from this tool.
	ok, err := t.taskService.UpdateTask(&current.ChecklistTask)
	return nil, ok, err
}

type MoveTaskStageInput struct {
	TaskID    int `json:"taskId"`
	FromStage int `json:"fromStage" jsonschema:"the stage id the caller currently believes the task is in"`
	ToStage   int `json:"toStage"`
}

func (t *toolset) moveTaskStage(ctx context.Context, req *mcp.CallToolRequest, in MoveTaskStageInput) (*mcp.CallToolResult, bool, error) {
	ok, err := t.taskService.TransitionStage(in.TaskID, in.FromStage, in.ToStage)
	if errors.Is(err, services.ErrStageConflict) {
		// Return this as a tool error with an actionable message, not a bare
		// conflict — the calling model should re-read the task and retry, not give up.
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{
			&mcp.TextContent{Text: "task is no longer in fromStage — call read_task again and retry with the current stage"},
		}}, false, nil
	}
	return nil, ok, err
}
