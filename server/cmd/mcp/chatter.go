package main

import (
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

func (t *toolset) projectContext(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := taskownership.CheckChat(t.taskService.TaskRepo.DB, t.runProjectID, t.runChatTurnID); err != nil {
		return nil, nil, err
	}
	value, err := (projectchat.Store{DB: t.taskService.TaskRepo.DB}).Context(t.runProjectID)
	return nil, value, err
}

type ReadDocumentInput struct {
	DocumentID int `json:"documentId"`
}

func (t *toolset) readProjectDocument(ctx context.Context, req *mcp.CallToolRequest, in ReadDocumentInput) (*mcp.CallToolResult, map[string]any, error) {
	d, err := (&knowledge.Store{DB: t.taskService.TaskRepo.DB, ProjectScope: t.runProjectID}).Document(t.runProjectID, in.DocumentID)
	if err != nil {
		return nil, nil, err
	}
	body, err := t.bodyToMarkdown(ctx, string(d.Body))
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"id": d.ID, "title": d.Title, "body": body, "file": d.File, "binaryContentsIncluded": false}, nil
}

type RequestTaskWorkInput struct {
	TaskID      int    `json:"taskId"`
	Intent      string `json:"intent" jsonschema:"ask, plan, implement, or review"`
	Instruction string `json:"instruction"`
	PresetID    int    `json:"presetId,omitempty"`
}

func (t *toolset) requestTaskWork(ctx context.Context, req *mcp.CallToolRequest, in RequestTaskWorkInput) (*mcp.CallToolResult, *models.AgentJob, error) {
	if t.aiService == nil {
		return nil, nil, fmt.Errorf("task dispatch is unavailable")
	}
	task, err := t.taskService.GetTask(in.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if task.ProjectID != t.runProjectID {
		return nil, nil, fmt.Errorf("task is outside this project")
	}
	// Slow provider/repository preparation occurs before the atomic ownership check.
	snapshot, err := t.aiService.Prepare(ctx, in.TaskID, services.AIRunInput{Intent: in.Intent, Instruction: in.Instruction, PresetID: in.PresetID})
	if err != nil {
		return nil, nil, err
	}
	if snapshot.ProjectID != t.runProjectID {
		return nil, nil, fmt.Errorf("task moved outside this project")
	}
	job, err := t.aiService.Jobs.EnqueueAI(in.TaskID, in.PresetID, *snapshot, t.runChatTurnID)
	return nil, job, err
}
