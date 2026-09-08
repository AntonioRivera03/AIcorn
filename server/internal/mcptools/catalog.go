package mcptools

import "github.com/modelcontextprotocol/go-sdk/mcp"

type Name string

const (
	SearchTasks        Name = "search_tasks"
	ReadTask           Name = "read_task"
	ListProjects       Name = "list_projects"
	ListWorkflowStages Name = "list_workflow_stages"
	CreateTask         Name = "create_task"
	UpdateTask         Name = "update_task"
	MoveTaskStage      Name = "move_task_stage"
	ListChecklists     Name = "list_checklists"
	ListTaskTypes      Name = "list_task_types"
)

type Definition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var orderedNames = [...]Name{
	SearchTasks,
	ReadTask,
	ListProjects,
	ListWorkflowStages,
	CreateTask,
	UpdateTask,
	MoveTaskStage,
	ListChecklists,
	ListTaskTypes,
}

var descriptions = map[Name]string{
	SearchTasks:        "Search and filter tasks across all projects. Task bodies are returned as markdown. Call list_workflow_stages first if filtering by stage.",
	ReadTask:           "Read a single task's full details by id. The body is returned as markdown.",
	ListProjects:       "List all projects.",
	ListWorkflowStages: "List all workflow stages. Use this to learn valid stage ids before calling search_tasks, create_task, or move_task_stage.",
	CreateTask:         "Create a new task on a checklist. The task body is left empty — write it through update_task or the app after creation.",
	UpdateTask:         "Update a task's name, priority, assignee, body (markdown), checklist, or type. Never changes stage — use move_task_stage for that.",
	MoveTaskStage:      "Move a task to a different workflow stage. Requires the stage you currently believe the task is in (fromStage); fails safely if the task has moved since you last read it.",
	ListChecklists:     "List all checklists, optionally filtered by project. Use this to learn valid checklist ids before calling create_task or update_task.",
	ListTaskTypes:      "List all task types. Use this to learn valid type ids before calling create_task or update_task.",
}

func All() []Definition {
	definitions := make([]Definition, 0, len(orderedNames))
	for _, name := range orderedNames {
		definitions = append(definitions, definition(name))
	}
	return definitions
}

func Tool(name Name) *mcp.Tool {
	tool := definition(name)
	result := &mcp.Tool{Name: tool.Name, Description: tool.Description}
	switch name {
	case SearchTasks, ReadTask, ListProjects, ListWorkflowStages, ListChecklists, ListTaskTypes:
		closedWorld := false
		result.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld}
	}
	return result
}

func definition(name Name) Definition {
	return Definition{Name: string(name), Description: descriptions[name]}
}
