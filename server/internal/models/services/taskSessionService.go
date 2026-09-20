package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/taskintent"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

type TaskMessageInput struct {
	Key           string `json:"key"`
	Message       string `json:"message"`
	PreviousJob   int    `json:"previousJob"`
	ExpectedStage int    `json:"expectedStage"`
	Decision      string `json:"decision"` // auto, question, or explicitly confirmed work
}
type TaskMessageResult struct {
	Job                  *models.AgentJob    `json:"job,omitempty"`
	ConfirmationRequired bool                `json:"confirmationRequired"`
	RequiresNewSession   bool                `json:"requiresNewSession,omitempty"`
	Intent               taskintent.Decision `json:"intent"`
}

func resumeCursor(latest *models.AgentJob, a models.AIRunArtifacts, key string) *models.ChatTurn {
	c := &models.ChatTurn{ClientKey: key}
	if latest == nil {
		return c
	}
	c.PreviousJob = latest.ID
	if latest.Request.Chat != nil {
		previous := *latest.Request.Chat
		c.SessionID, c.Workspace, c.Branch, c.BaseCommit = previous.SessionID, previous.Workspace, previous.Branch, previous.BaseCommit
	}
	if a.SessionID != "" {
		c.SessionID = a.SessionID
	}
	if a.Workspace != "" {
		c.Workspace, c.Branch, c.BaseCommit = a.Workspace, a.Branch, a.BaseCommit
	}
	return c
}

func (s *AIService) TaskMessage(ctx context.Context, taskID int, in TaskMessageInput) (TaskMessageResult, error) {
	result := TaskMessageResult{}
	in.Message = strings.TrimSpace(in.Message)
	if in.Key == "" || len(in.Key) > 128 || in.Message == "" || len(in.Message) > 32000 || (in.Decision != "auto" && in.Decision != "question" && in.Decision != "work") {
		return result, ErrInvalidAIRun
	}
	if prior, err := s.Jobs.ChatRequest(taskID, in.Key); err != nil || prior != nil {
		result.Job = prior
		return result, err
	}
	task, err := s.Tasks.FindOneWithProject(taskID)
	if err != nil {
		return result, err
	}
	if task.Stage != in.ExpectedStage {
		return result, repos.ErrChatConflict
	}
	if err = taskownership.Check(s.Jobs.DB, taskID, 0); err != nil {
		return result, err
	}
	latest, artifacts, err := s.Jobs.LatestChat(taskID)
	if err != nil {
		return result, err
	}
	if latest == nil || latest.Request.TaskSession == nil || latest.ID != in.PreviousJob {
		return result, repos.ErrChatConflict
	}
	if artifacts.SessionID == "" && latest.Request.Chat.SessionID == "" {
		if in.Decision == "question" {
			return result, fmt.Errorf("%w: the previous run failed before a conversation was established; confirm Resume work to start it", ErrInvalidAIRun)
		}
		if in.Decision == "auto" {
			result.ConfirmationRequired = true
			result.RequiresNewSession = true
			result.Intent = taskintent.Decision{Intent: "work", Provider: "manual", Reason: "The previous run did not establish a conversation. Resume work will start this task's session."}
			return result, nil
		}
	}
	decision := taskintent.Decision{Intent: in.Decision, Provider: "user"}
	if in.Decision == "auto" {
		classifier := s.IntentClassifier
		if classifier == nil {
			classifier = taskintent.FromEnvironment()
		}
		body, err := s.Converter.ToMarkdown(ctx, []string{task.Body})
		if err != nil {
			return result, err
		}
		var answer string
		_ = s.Jobs.DB.QueryRow(`SELECT output FROM agent_run WHERE job=? ORDER BY id DESC LIMIT 1`, latest.ID).Scan(&answer)
		decision = classifier.Classify(ctx, taskintent.Context{Title: task.Name, Description: body[0], LastRequest: latest.Request.Instruction, LastAnswer: answer, Message: in.Message})
		if decision.Intent != "question" {
			result.ConfirmationRequired = true
			result.Intent = decision
			return result, nil
		}
	}
	prior := latest.Request
	role := prior.TaskSession.Role
	// Retain the task's selected agent and model throughout its conversation.
	agent := &models.AgentSnapshot{ID: prior.AgentID, Name: prior.PresetName, Model: prior.Model, Instructions: prior.SystemPrompt}
	intent := "ask"
	if decision.Intent == "work" && role == "coder" && prior.RepoPath != "" {
		intent = "implement"
	}
	req, err := s.Prepare(ctx, taskID, AIRunInput{Intent: intent, Agent: agent, Instruction: in.Message, UseRepository: prior.RepoPath != ""})
	if err != nil {
		return result, err
	}
	if req.RepoPath != prior.RepoPath {
		return result, fmt.Errorf("%w: the linked repository changed; start a new task run", ErrInvalidAIRun)
	}
	req.Chat = resumeCursor(latest, artifacts, in.Key)
	req.TaskSession = &models.TaskSession{Role: role, Mode: decision.Intent, Settings: prior.TaskSession.Settings, ExpectedStage: task.Stage}
	if decision.Intent == "work" && req.TaskSession.Settings != nil {
		settings := *req.TaskSession.Settings
		task.Body, err = s.Tasks.RawTaskBody(taskID)
		if err != nil {
			return result, err
		}
		req.Conductor = &models.ConductorRun{Independent: true, Phase: "working", Settings: settings, SourceBody: task.Body, TaskAgent: agent}
	}
	result.Job, err = s.Jobs.EnqueueChat(taskID, *req)
	result.Intent = decision
	return result, err
}
