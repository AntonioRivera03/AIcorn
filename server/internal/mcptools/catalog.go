package mcptools

import "github.com/modelcontextprotocol/go-sdk/mcp"

type Name string

const (
	ProjectContext      Name = "project_context"
	ReadProjectDocument Name = "read_project_document"
	RequestTaskWork     Name = "request_task_work"
	SearchTasks         Name = "search_tasks"
	ReadTask            Name = "read_task"
	ListProjects        Name = "list_projects"
	ListWorkflowStages  Name = "list_workflow_stages"
	CreateTask          Name = "create_task"
	UpdateTask          Name = "update_task"
	MoveTaskStage       Name = "move_task_stage"
	ListChecklists      Name = "list_checklists"
	ListTaskTypes       Name = "list_task_types"
	ListTaskLinks       Name = "list_task_links"
	AddTaskLink         Name = "add_task_link"
	RemoveTaskLink      Name = "remove_task_link"
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
	ListTaskLinks,
	AddTaskLink,
	RemoveTaskLink,
}

var descriptions = map[Name]string{
	ProjectContext:      "Read this project’s actual workflow, stages, checklists, task types, Conductor settings, active task owners, and document metadata.",
	ReadProjectDocument: "Read a project document’s written notes and original-file metadata. Binary file contents are not included.",
	RequestTaskWork:     "Queue a task agent to work on a free task in this project. Implement/review requires a linked repository. Active owners block dispatch; queued does not mean completed.",
	SearchTasks:         "Search and filter accessible tasks. Agent runs are limited to their assigned project. Bodies are returned as markdown. Discover stage IDs through project_context in scoped runs, or list_workflow_stages in interactive sessions.",
	ReadTask:            "Read a single task's full details by id. The body is returned as markdown.",
	ListProjects:        "List all projects.",
	ListWorkflowStages:  "List all workflow stages. Use this to learn valid stage ids before calling search_tasks, create_task, or move_task_stage.",
	CreateTask:          "Create a new task on a checklist. The task body is left empty — write it through update_task or the app after creation.",
	UpdateTask:          "Update a task's name, priority, assignee, body (markdown), checklist, or type. Never changes stage — use move_task_stage for that.",
	MoveTaskStage:       "Move a task to a different workflow stage. Requires the stage you currently believe the task is in (fromStage); fails safely if the task has moved since you last read it.",
	ListChecklists:      "List all checklists, optionally filtered by project. Use this to learn valid checklist ids before calling create_task or update_task.",
	ListTaskTypes:       "List all task types. Use this to learn valid type ids before calling create_task or update_task.",
	ListTaskLinks:       "List GitHub pull request and branch references attached to a task, including their IDs and revisions.",
	AddTaskLink:         "Attach a GitHub pull request or branch URL to a task with an optional label. Repeating the same URL returns the existing link. Other agents' active tasks are protected.",
	RemoveTaskLink:      "Remove a GitHub reference from a task using its ID and revision. This does not change anything on GitHub. Other agents' active tasks are protected.",
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
	case ProjectContext, ReadProjectDocument, SearchTasks, ReadTask, ListProjects, ListWorkflowStages, ListChecklists, ListTaskTypes, ListTaskLinks:
		closedWorld := false
		result.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld}
	}
	return result
}

func definition(name Name) Definition {
	return Definition{Name: string(name), Description: descriptions[name]}
}
