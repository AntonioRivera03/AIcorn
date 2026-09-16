package services

import (
	"context"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"strings"
)

type ChatInput struct {
	Key             string `json:"key"`
	Message         string `json:"message"`
	PresetID        int    `json:"presetId"`
	Mode            string `json:"mode"` // ask is read-only; edit uses an isolated repository.
	UseRepository   bool   `json:"useRepository"`
	NewConversation bool   `json:"newConversation"`
}

func (s *AIService) StartChat(ctx context.Context, taskID int, in ChatInput) (*models.AgentJob, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.Key == "" || len(in.Key) > 128 || in.Message == "" || len(in.Message) > 32000 || (in.Mode != "ask" && in.Mode != "edit") {
		return nil, fmt.Errorf("%w: provide a message, request key and ask/edit mode", ErrInvalidAIRun)
	}
	if prior, err := s.Jobs.ChatRequest(taskID, in.Key); err != nil || prior != nil {
		return prior, err
	}
	task, err := s.Tasks.FindOneWithProject(taskID)
	if err != nil {
		return nil, err
	}
	if task.Type.ViewMode != "chat" {
		return nil, repos.ErrChatType
	}
	latest, artifacts, err := s.Jobs.LatestChat(taskID)
	if err != nil {
		return nil, err
	}
	chat := &models.ChatTurn{ClientKey: in.Key}
	if latest != nil {
		chat.PreviousJob = latest.ID
		switch latest.Status {
		case "pending", "claimed", "running", "canceling":
			return nil, repos.ErrActiveAIRun
		}
		if !in.NewConversation {
			previous := latest.Request.Chat
			chat.SessionID, chat.Workspace, chat.Branch, chat.BaseCommit = previous.SessionID, previous.Workspace, previous.Branch, previous.BaseCommit
			if artifacts.SessionID != "" {
				chat.SessionID = artifacts.SessionID
			}
			if artifacts.Workspace != "" {
				chat.Workspace, chat.Branch, chat.BaseCommit = artifacts.Workspace, artifacts.Branch, artifacts.BaseCommit
			}
		}
	}
	intent := "ask"
	if in.Mode == "edit" {
		intent = "implement"
		in.UseRepository = true
	}
	// Hidden document content is preserved on type changes but not sent to a chat.
	task.Body = models.EmptyBody
	req, err := s.PrepareSnapshot(ctx, task, AIRunInput{Intent: intent, Instruction: in.Message, PresetID: in.PresetID, UseRepository: in.UseRepository})
	if err != nil {
		return nil, err
	}
	if latest != nil && !in.NewConversation && latest.Request.RepoPath != req.RepoPath {
		return nil, fmt.Errorf("%w: repository context changed; start a new conversation to use it", ErrInvalidAIRun)
	}
	req.Chat = chat
	return s.Jobs.EnqueueChat(taskID, *req)
}
