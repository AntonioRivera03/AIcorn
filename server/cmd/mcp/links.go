package main

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
)

type TaskLinksOutput struct {
	Links []knowledge.TaskLink `json:"links"`
}
type AddTaskLinkInput struct {
	TaskID int    `json:"taskId"`
	URL    string `json:"url" jsonschema:"https://github.com/owner/repo/pull/123 or https://github.com/owner/repo/tree/branch"`
	Label  string `json:"label,omitempty"`
}
type RemoveTaskLinkInput struct {
	TaskID   int `json:"taskId"`
	LinkID   int `json:"linkId"`
	Revision int `json:"revision" jsonschema:"current revision from list_task_links"`
}

func (t *toolset) linkScope(task int, write bool) error {
	if t.runTaskID > 0 && (write || t.runProjectID == 0) && task != t.runTaskID {
		return errors.New("this run may only access links on its own task")
	}
	if write && t.runTaskID > 0 && t.runJobID <= 0 {
		return errors.New("this run has no task ownership identity")
	}
	current, err := t.taskService.GetTask(task)
	if err != nil {
		return err
	}
	if t.runProjectID > 0 && current.ProjectID != t.runProjectID {
		return errors.New("task is outside this project's scope")
	}
	return nil
}
func (t *toolset) listTaskLinks(ctx context.Context, req *mcp.CallToolRequest, in ReadTaskInput) (*mcp.CallToolResult, TaskLinksOutput, error) {
	if err := t.linkScope(in.TaskID, false); err != nil {
		return nil, TaskLinksOutput{}, err
	}
	links, err := (&knowledge.Store{DB: t.taskService.TaskRepo.DB, ProjectScope: t.runProjectID, ChatTurnID: t.runChatTurnID}).Links(in.TaskID)
	return nil, TaskLinksOutput{Links: links}, err
}
func (t *toolset) addTaskLink(ctx context.Context, req *mcp.CallToolRequest, in AddTaskLinkInput) (*mcp.CallToolResult, knowledge.TaskLink, error) {
	if err := t.linkScope(in.TaskID, true); err != nil {
		return nil, knowledge.TaskLink{}, err
	}
	link, err := (&knowledge.Store{DB: t.taskService.TaskRepo.DB, ProjectScope: t.runProjectID, ChatTurnID: t.runChatTurnID}).PutLink(in.TaskID, 0, knowledge.LinkInput{URL: in.URL, Label: in.Label}, &t.runJobID)
	return nil, link, err
}
func (t *toolset) removeTaskLink(ctx context.Context, req *mcp.CallToolRequest, in RemoveTaskLinkInput) (*mcp.CallToolResult, OkOutput, error) {
	if err := t.linkScope(in.TaskID, true); err != nil {
		return nil, OkOutput{}, err
	}
	err := (&knowledge.Store{DB: t.taskService.TaskRepo.DB, ProjectScope: t.runProjectID, ChatTurnID: t.runChatTurnID}).DeleteLink(in.TaskID, in.LinkID, in.Revision, &t.runJobID)
	return nil, OkOutput{Ok: err == nil}, err
}
