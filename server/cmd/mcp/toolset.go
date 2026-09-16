package main

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/mcptools"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func parseRFC3339(value string) (*time.Time, error) {
	timestamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &timestamp, nil
}

type toolset struct {
	runProjectID     int
	runTaskID        int
	runJobID         int
	taskService      *services.TaskService
	projectService   *services.ProjectService
	stageService     *services.StageService
	checklistService *services.ChecklistService
	taskTypeService  *services.TaskTypeService
	converter        *markdown.Converter
}

func (toolset *toolset) bodiesToMarkdown(ctx context.Context, tasks []models.TaskWithProject) error {
	bodies := make([]string, len(tasks))
	for index, task := range tasks {
		bodies[index] = task.Body
	}
	markdowns, err := toolset.converter.ToMarkdown(ctx, bodies)
	if err != nil {
		return err
	}
	for index := range tasks {
		tasks[index].Body = markdowns[index]
	}
	return nil
}

func (toolset *toolset) bodyToMarkdown(ctx context.Context, body string) (string, error) {
	markdowns, err := toolset.converter.ToMarkdown(ctx, []string{body})
	if err != nil {
		return "", err
	}
	return markdowns[0], nil
}

func (toolset *toolset) bodyToBody(ctx context.Context, markdownBody string) (string, error) {
	bodies, err := toolset.converter.ToBody(ctx, []string{markdownBody})
	if err != nil {
		return "", err
	}
	return bodies[0], nil
}

func (toolset *toolset) register(server *mcp.Server) {
	if toolset.runTaskID == 0 || toolset.runJobID > 0 {
		mcp.AddTool(server, mcptools.Tool(mcptools.ListTaskLinks), toolset.listTaskLinks)
		mcp.AddTool(server, mcptools.Tool(mcptools.AddTaskLink), toolset.addTaskLink)
		mcp.AddTool(server, mcptools.Tool(mcptools.RemoveTaskLink), toolset.removeTaskLink)
	}
	if toolset.runProjectID > 0 {
		mcp.AddTool(server, mcptools.Tool(mcptools.ReadTask), toolset.readTask)
		mcp.AddTool(server, mcptools.Tool(mcptools.SearchTasks), toolset.searchTasks)
		return
	}
	if toolset.runTaskID > 0 {
		mcp.AddTool(server, mcptools.Tool(mcptools.ReadTask), toolset.readTask)
		return
	}
	mcp.AddTool(server, mcptools.Tool(mcptools.SearchTasks), toolset.searchTasks)
	mcp.AddTool(server, mcptools.Tool(mcptools.ReadTask), toolset.readTask)
	mcp.AddTool(server, mcptools.Tool(mcptools.ListProjects), toolset.listProjects)
	mcp.AddTool(server, mcptools.Tool(mcptools.ListWorkflowStages), toolset.listWorkflowStages)
	mcp.AddTool(server, mcptools.Tool(mcptools.CreateTask), toolset.createTask)
	mcp.AddTool(server, mcptools.Tool(mcptools.UpdateTask), toolset.updateTask)
	mcp.AddTool(server, mcptools.Tool(mcptools.MoveTaskStage), toolset.moveTaskStage)
	mcp.AddTool(server, mcptools.Tool(mcptools.ListChecklists), toolset.listChecklists)
	mcp.AddTool(server, mcptools.Tool(mcptools.ListTaskTypes), toolset.listTaskTypes)
}
