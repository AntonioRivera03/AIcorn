package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
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
	converter      *markdown.Converter
}

// bodiesToMarkdown rewrites each task's stored Plate.js document into markdown
// in place, so a calling agent reads prose instead of a JSON tree. A conversion
// failure is returned, never swallowed — handing back raw Plate JSON under the
// guise of markdown would silently corrupt whatever the agent writes back.
func (t *toolset) bodiesToMarkdown(ctx context.Context, tasks []models.TaskWithProject) error {
	bodies := make([]string, len(tasks))
	for i, task := range tasks {
		bodies[i] = task.Body
	}
	markdowns, err := t.converter.ToMarkdown(ctx, bodies)
	if err != nil {
		return err
	}
	for i := range tasks {
		tasks[i].Body = markdowns[i]
	}
	return nil
}

// bodyToMarkdown and bodyToBody are the single-item form of bodiesToMarkdown /
// Converter.ToBody, for the two call sites (readTask, updateTask) that only
// ever have one body to convert — avoids each hand-rolling its own
// wrap-in-a-slice/unwrap-index-0 around the batch API.
func (t *toolset) bodyToMarkdown(ctx context.Context, body string) (string, error) {
	markdowns, err := t.converter.ToMarkdown(ctx, []string{body})
	if err != nil {
		return "", err
	}
	return markdowns[0], nil
}

func (t *toolset) bodyToBody(ctx context.Context, markdown string) (string, error) {
	bodies, err := t.converter.ToBody(ctx, []string{markdown})
	if err != nil {
		return "", err
	}
	return bodies[0], nil
}

func (t *toolset) register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_tasks",
		Description: "Search and filter tasks across all projects. Task bodies are returned as markdown. Call list_workflow_stages first if filtering by stage.",
	}, t.searchTasks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_task",
		Description: "Read a single task's full details by id. The body is returned as markdown.",
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
		Description: "Update a task's name, priority, assignee, or body (markdown). Never changes stage — use move_task_stage for that.",
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

// OkOutput wraps boolean results (update_task, move_task_stage) — same rule as
// above: structuredContent must be an object.
type OkOutput struct {
	Ok bool `json:"ok"`
}

func (t *toolset) searchTasks(ctx context.Context, req *mcp.CallToolRequest, in SearchTasksInput) (*mcp.CallToolResult, TasksOutput, error) {
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
	task, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, nil, err
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
	TaskID   int     `json:"taskId"`
	Name     *string `json:"name,omitempty"`
	Priority *string `json:"priority,omitempty" jsonschema:"Urgent, High, Medium, or Low"`
	Assignee *string `json:"assignee,omitempty"`
	Body     *string `json:"body,omitempty" jsonschema:"markdown — headings, lists, tables, code blocks and task checkboxes are all supported. Overwrites the existing body."`
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
