package main

import (
	"context"
	"errors"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

type SearchTasksInput struct {
	Query      string   `json:"query,omitempty" jsonschema:"free-text search over the task name"`
	ProjectIDs []int    `json:"projectIds,omitempty"`
	StageIDs   []int    `json:"stageIds,omitempty" jsonschema:"stage ids — call list_workflow_stages for valid ids"`
	Priorities []string `json:"priorities,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	Assignees  []string `json:"assignees,omitempty"`
	Limit      int      `json:"limit,omitempty" jsonschema:"default 25, max 100"`
}

// MCP structuredContent must be a JSON object, never an array — list results
// are wrapped in these single-field structs.
type TasksOutput struct {
	Tasks []models.TaskWithProject `json:"tasks"`
}

type ProjectsOutput struct {
	Projects []models.Project `json:"projects"`
}

type StagesOutput struct {
	Stages []models.Stage `json:"stages"`
}

type ChecklistsOutput struct {
	Checklists []models.Checklist `json:"checklists"`
}

type TaskTypesOutput struct {
	TaskTypes []models.TaskType `json:"taskTypes"`
}

// OkOutput wraps boolean results (update_task, move_task_stage) — same rule as
// above: structuredContent must be an object.
type OkOutput struct {
	Ok bool `json:"ok"`
}

func (t *toolset) searchTasks(ctx context.Context, req *mcp.CallToolRequest, in SearchTasksInput) (*mcp.CallToolResult, TasksOutput, error) {
	if t.runProjectID > 0 {
		in.ProjectIDs = []int{t.runProjectID}
	}
	filters := &repos.TaskFilters{
		SearchQuery:    in.Query,
		ProjectIDQuery: in.ProjectIDs,
		PriorityQuery:  in.Priorities,
		AssigneeQuery:  in.Assignees,
	}
	for _, id := range in.StageIDs {
		filters.StageQuery = append(filters.StageQuery, strconv.Itoa(id))
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	filters.Limit = limit

	tasks, err := t.taskService.GetAllTasks(filters)
	if err != nil {
		return nil, TasksOutput{}, err
	}

	if err := t.bodiesToMarkdown(ctx, tasks); err != nil {
		return nil, TasksOutput{}, err
	}
	return nil, TasksOutput{Tasks: tasks}, nil
}

type ReadTaskInput struct {
	TaskID int `json:"taskId"`
}

func (t *toolset) readTask(ctx context.Context, req *mcp.CallToolRequest, in ReadTaskInput) (*mcp.CallToolResult, *models.TaskWithProject, error) {
	if t.runProjectID == 0 && t.runTaskID > 0 && in.TaskID != t.runTaskID {
		return nil, nil, errors.New("this run may only read its own task")
	}
	task, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if t.runProjectID > 0 && task.ProjectID != t.runProjectID {
		return nil, nil, errors.New("Conductor may only read tasks in its project")
	}

	body, err := t.bodyToMarkdown(ctx, task.Body)
	if err != nil {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	task.Body = body
	return nil, task, nil
}

type NoInput struct{}

func (t *toolset) listProjects(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, ProjectsOutput, error) {
	projects, err := t.projectService.GetAllProjects()
	if err != nil {
		return nil, ProjectsOutput{}, err
	}
	return nil, ProjectsOutput{Projects: projects}, nil
}

func (t *toolset) listWorkflowStages(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, StagesOutput, error) {
	stages, err := t.stageService.GetAllStages()
	if err != nil {
		return nil, StagesOutput{}, err
	}
	return nil, StagesOutput{Stages: stages}, nil
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
// ChecklistTask body. This tool writes through TaskService.UpdateTaskProperties,
// which has no stage column in its UPDATE at all — unlike TaskService.UpdateTask
// (used by PUT /api/task), it's not possible for this tool to move a task
// sideways of move_task_stage's CAS check, even by accident.
type UpdateTaskInput struct {
	TaskID      int     `json:"taskId"`
	Name        *string `json:"name,omitempty"`
	Priority    *string `json:"priority,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	Assignee    *string `json:"assignee,omitempty"`
	Body        *string `json:"body,omitempty" jsonschema:"markdown — headings, lists, tables, code blocks and task checkboxes are all supported. Overwrites the existing body."`
	ChecklistID *int    `json:"checklistId,omitempty" jsonschema:"move the task to a different checklist — call list_checklists for valid ids"`
	TypeID      *int    `json:"typeId,omitempty" jsonschema:"change the task's type — call list_task_types for valid ids"`
}

func (t *toolset) updateTask(ctx context.Context, req *mcp.CallToolRequest, in UpdateTaskInput) (*mcp.CallToolResult, OkOutput, error) {
	current, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, OkOutput{}, err
	}

	// Convert before writing anything: a malformed body should leave the task
	// entirely untouched rather than half-applying the property changes.
	body := ""
	if in.Body != nil {
		converted, err := t.bodyToBody(ctx, *in.Body)
		if err != nil {
			return nil, OkOutput{}, err
		}
		body = converted
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
	if in.ChecklistID != nil {
		if t.checklistService == nil || t.checklistService.ChecklistRepo == nil {
			return nil, OkOutput{}, errors.New("checklist service not configured")
		}
		ch, err := t.checklistService.ChecklistRepo.FindOne(int64(*in.ChecklistID))
		if err != nil {
			return nil, OkOutput{}, err
		}
		current.Checklist = ch.ID
	}
	if in.TypeID != nil {
		if t.taskTypeService == nil || t.taskTypeService.TaskTypeRepo == nil {
			return nil, OkOutput{}, errors.New("task type service not configured")
		}
		tt, err := t.taskTypeService.TaskTypeRepo.FindOne(*in.TypeID)
		if err != nil {
			return nil, OkOutput{}, err
		}
		current.Type = *tt
	}
	ok, err := t.taskService.UpdateTaskProperties(&current.ChecklistTask)
	if err != nil || !ok {
		return nil, OkOutput{Ok: ok}, err
	}

	if in.Body != nil {
		// Written through UpdateTaskBody, not the UpdateTask call above — same
		// separation the app itself relies on (taskRepo.go's UpdateTask never
		// touches the body column, precisely so a property-only edit can't
		// clobber it).
		ok, err = t.taskService.UpdateTaskBody(in.TaskID, body)
	}
	return nil, OkOutput{Ok: ok}, err
}

type MoveTaskStageInput struct {
	TaskID    int `json:"taskId"`
	FromStage int `json:"fromStage" jsonschema:"the stage id the caller currently believes the task is in"`
	ToStage   int `json:"toStage"`
}

func (t *toolset) moveTaskStage(ctx context.Context, req *mcp.CallToolRequest, in MoveTaskStageInput) (*mcp.CallToolResult, OkOutput, error) {
	ok, err := t.taskService.TransitionStage(in.TaskID, in.FromStage, in.ToStage)
	if errors.Is(err, services.ErrStageConflict) {
		// Return this as a tool error with an actionable message, not a bare
		// conflict — the calling model should re-read the task and retry, not give up.
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{
			&mcp.TextContent{Text: "task is no longer in fromStage — call read_task again and retry with the current stage"},
		}}, OkOutput{}, nil
	}
	return nil, OkOutput{Ok: ok}, err
}

type ListChecklistsInput struct {
	ProjectID *int `json:"projectId,omitempty"`
}

func (t *toolset) listChecklists(ctx context.Context, req *mcp.CallToolRequest, in ListChecklistsInput) (*mcp.CallToolResult, ChecklistsOutput, error) {
	checklists, err := t.checklistService.ListChecklists(in.ProjectID)
	if err != nil {
		return nil, ChecklistsOutput{}, err
	}
	if checklists == nil {
		checklists = []models.Checklist{}
	}
	return nil, ChecklistsOutput{Checklists: checklists}, nil
}

func (t *toolset) listTaskTypes(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, TaskTypesOutput, error) {
	types, err := t.taskTypeService.TaskTypeRepo.All()
	if err != nil {
		return nil, TaskTypesOutput{}, err
	}
	if types == nil {
		types = []models.TaskType{}
	}
	return nil, TaskTypesOutput{TaskTypes: types}, nil
}
