package main

import (
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

func (t *toolset) projectContext(ctx context.Context, req *mcp.CallToolRequest, in NoInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := t.checkProjectReadScope(); err != nil {
		return nil, nil, err
	}
	value, err := (projectchat.Store{DB: t.taskService.TaskRepo.DB}).Context(t.runProjectID)
	return nil, value, err
}

// Both Chatter and Conductor can inspect their project. Recheck the binding on
// each call so moving the assigned task cannot leave stale cross-project access.
func (t *toolset) checkProjectReadScope() error {
	if t.runProjectID <= 0 {
		return fmt.Errorf("project context requires a scoped agent run")
	}
	if t.runDispatchID > 0 {
		return repos.CheckDispatch(t.taskService.TaskRepo.DB, t.runProjectID, t.runDispatchID)
	}
	if t.runChatTurnID > 0 {
		return taskownership.CheckChat(t.taskService.TaskRepo.DB, t.runProjectID, t.runChatTurnID)
	}
	if t.runTaskID <= 0 {
		return fmt.Errorf("project context requires an assigned task")
	}
	task, err := t.taskService.GetTask(t.runTaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != t.runProjectID {
		return fmt.Errorf("assigned task is outside this project")
	}
	return nil
}

type ReadDocumentInput struct {
	DocumentID int `json:"documentId"`
}

func (t *toolset) readProjectDocument(ctx context.Context, req *mcp.CallToolRequest, in ReadDocumentInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := t.checkProjectReadScope(); err != nil {
		return nil, nil, err
	}
	d, err := (&knowledge.Store{DB: t.taskService.TaskRepo.DB, ProjectScope: t.runProjectID}).Document(t.runProjectID, in.DocumentID)
	if err != nil {
		return nil, nil, err
	}
	body, err := t.bodyToMarkdown(ctx, string(d.Body))
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"id": d.ID, "title": d.Title, "tags": d.Tags, "details": d.Details, "body": body, "file": d.File, "binaryContentsIncluded": false}, nil
}

type SendToConductorInput struct {
	TaskIDs []int `json:"taskIds" jsonschema:"IDs of tasks in this project to hand to Conductor"`
}

type SendToConductorOutput struct {
	TaskIDs []int `json:"taskIds"`
	Sent    int   `json:"sent"`
	Skipped int   `json:"skipped"`
	Failed  int   `json:"failed"`
	// ConductorRunning is false while Conductor is paused: sent tasks wait
	// until someone starts it.
	ConductorRunning bool `json:"conductorRunning"`
}

// sendToConductor is how Chatter gets work done without doing it: Conductor
// picks the agent and runs it in its own session.
func (t *toolset) sendToConductor(ctx context.Context, req *mcp.CallToolRequest, in SendToConductorInput) (*mcp.CallToolResult, SendToConductorOutput, error) {
	out := SendToConductorOutput{TaskIDs: in.TaskIDs}
	if err := t.checkProjectReadScope(); err != nil {
		return nil, out, err
	}
	if t.conductorService == nil {
		return nil, out, fmt.Errorf("Conductor is unavailable")
	}
	result, err := t.conductorService.Manage(t.runProjectID, in.TaskIDs, "send")
	if err != nil {
		return nil, out, err
	}
	out.Sent, out.Skipped, out.Failed = result.Success, result.Skipped, result.Failed
	if settings, _, err := t.conductorService.Repo.Settings(t.runProjectID); err == nil {
		out.ConductorRunning = settings.Enabled
	}
	return nil, out, nil
}
