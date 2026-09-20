package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

// DispatchCandidate includes the actual task and durable owner state. Only
// tasks explicitly handed to Conductor are eligible for its start tool.
type DispatchCandidate struct {
	State models.ConductorTask    `json:"state"`
	Task  *models.TaskWithProject `json:"task"`
}

func (s *ConductorService) Candidates(ctx context.Context, project int) ([]DispatchCandidate, error) {
	tasks, err := s.Repo.Tasks(project)
	if err != nil {
		return nil, err
	}
	result := []DispatchCandidate{}
	for _, t := range tasks {
		task, err := s.AI.Tasks.FindOneWithProject(t.TaskID)
		if err != nil {
			return nil, err
		}
		if task.ProjectID != project {
			continue
		}
		body, err := s.AI.Converter.ToMarkdown(ctx, []string{task.Body})
		if err != nil {
			return nil, err
		}
		task.Body = body[0]
		result = append(result, DispatchCandidate{State: t, Task: task})
	}
	return result, nil
}

// StartTask freezes a role/model and commits ownership, stage and queue entry
// together. MCP cannot supply a session, workspace, model or system prompt.
func (s *ConductorService) StartTask(ctx context.Context, project, taskID, dispatch int, role string) (*models.AgentJob, error) {
	if err := repos.CheckDispatch(s.Repo.DB, project, dispatch); err != nil {
		return nil, err
	}
	switch role {
	case "coder", "researcher", "reviewer", "planner":
	default:
		return nil, fmt.Errorf("%w: choose coder, researcher, reviewer, or planner", ErrInvalidAIRun)
	}
	if prior, err := s.Repo.StartedTask(project, taskID); err != nil || prior != nil {
		return prior, err
	}
	t, err := s.Repo.Task(project, taskID)
	if err != nil {
		return nil, err
	}
	if t.State != "waiting" {
		return nil, repos.ErrConductorConflict
	}
	task, err := s.AI.Tasks.FindOneWithProject(taskID)
	if err != nil {
		return nil, err
	}
	task.Body, err = s.AI.Tasks.RawTaskBody(taskID)
	if err != nil {
		return nil, err
	}
	settings, _, err := s.Repo.Settings(project)
	if err != nil {
		return nil, err
	}
	agent, err := s.AI.ResolveRole(ctx, role)
	if err != nil {
		return nil, err
	}
	intent := "ask"
	if role == "coder" && settings.UseRepository {
		intent = "implement"
	}
	req, err := s.AI.Prepare(ctx, taskID, AIRunInput{Intent: intent, Agent: agent, UseRepository: settings.UseRepository, Instruction: settings.WorkingPrompt})
	if err != nil {
		return nil, err
	}
	latest, artifacts, err := s.AI.Jobs.LatestChat(taskID)
	if err != nil {
		return nil, err
	}
	req.Chat = &models.ChatTurn{ClientKey: req.Key}
	if latest != nil && latest.Request.TaskSession != nil && latest.Request.RepoPath == req.RepoPath && latest.Request.TaskSession.Role == role {
		req.Chat = resumeCursor(latest, artifacts, req.Key)
	}
	req.TaskSession = &models.TaskSession{Role: role, Mode: "work", Settings: &settings, ExpectedStage: settings.WorkingStage}
	req.Conductor = &models.ConductorRun{Phase: "working", Settings: settings, SourceBody: task.Body, TaskAgent: agent}
	err = s.Repo.Advance(t, task, task.Body, settings.WorkingStage, "queued", "Task session queued", req, settings, dispatch)
	if errors.Is(err, repos.ErrConductorConflict) || errors.Is(err, repos.ErrActiveAIRun) {
		if prior, e := s.Repo.StartedTask(project, taskID); e == nil && prior != nil {
			return prior, nil
		}
	}
	if err != nil {
		return nil, err
	}
	updated, err := s.Repo.Task(project, taskID)
	if err != nil {
		return nil, err
	}
	return s.AI.Jobs.FindOne(updated.JobID)
}

// A project selection turn is short-lived and separate from every task's durable
// session. It uses only the dispatcher MCP surface, with native delegation off.
func (s *ConductorService) PrepareDispatch(ctx context.Context) (*models.AIRunRequest, []models.ConductorTask, error) {
	tasks, err := s.Repo.Tasks(0)
	if err != nil {
		return nil, nil, err
	}
	for _, t := range tasks {
		if t.State != "waiting" {
			continue
		}
		settings, _, err := s.Repo.Settings(t.ProjectID)
		if err != nil {
			return nil, nil, err
		}
		if !settings.Enabled {
			continue
		}
		if err = s.Repo.ValidateStages(t.ProjectID, settings); err != nil {
			_ = s.Repo.SetState(t, "held", err.Error())
			continue
		}
		var projectName string
		if err = s.Repo.DB.QueryRow("SELECT COALESCE(name,'') FROM project WHERE id=?", t.ProjectID).Scan(&projectName); err != nil {
			return nil, nil, err
		}
		snapshot := &models.TaskWithProject{ProjectID: t.ProjectID}
		snapshot.Name = projectName
		snapshot.Body = models.EmptyBody
		req, err := s.AI.PrepareSnapshot(ctx, snapshot, AIRunInput{Intent: "plan", PresetID: settings.ConductorAgentID, Instruction: settings.PlanningPrompt})
		if err != nil {
			_ = s.Repo.SetState(t, "failed", err.Error())
			return nil, nil, err
		}
		selected := []models.ConductorTask{}
		for _, other := range tasks {
			if other.ProjectID == t.ProjectID && other.State == "waiting" {
				selected = append(selected, other)
			}
		}
		id, err := s.Repo.BeginDispatch(t.ProjectID, req)
		if errors.Is(err, repos.ErrActiveAIRun) || errors.Is(err, repos.ErrConductorPaused) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		req.DispatchID = id
		req.TimeoutSeconds = min(req.TimeoutSeconds, 180)
		req.Instruction = strings.TrimSpace(req.Instruction) + "\nInspect conductor-managed tasks with list_conductor_tasks. Choose appropriate independent agent roles and call start_conductor_task for eligible work. Defer tasks with genuine blockers using defer_conductor_task and a specific reason. Starting queues an independent session; do not wait for it or claim its work is complete."
		return req, selected, nil
	}
	return nil, nil, nil
}

// Explicit rechecks of scheduled Jobs keep their frozen agent and workflow even
// while the board dispatcher is paused. They still resume a single task session.
func (s *ConductorService) restartIndependent(ctx context.Context, t models.ConductorTask, task *models.TaskWithProject, previous *models.AgentJob) error {
	c := previous.Request.Conductor
	agent := c.TaskAgent
	if agent == nil {
		return fmt.Errorf("task agent is missing")
	}
	req, err := s.AI.Prepare(ctx, t.TaskID, AIRunInput{Intent: previous.Request.Intent, Agent: agent, UseRepository: c.Settings.UseRepository, Instruction: c.Settings.WorkingPrompt})
	if err != nil {
		return err
	}
	if req.RepoPath != previous.Request.RepoPath {
		return fmt.Errorf("%w: the linked repository changed; start a new task run", ErrInvalidAIRun)
	}
	if req.Intent == "plan" {
		req.Intent = "ask"
		if c.Settings.UseRepository {
			req.Intent = "implement"
		}
	}
	role := "coder"
	if previous.Request.TaskSession != nil {
		role = previous.Request.TaskSession.Role
	}
	latest, artifacts, err := s.AI.Jobs.LatestChat(t.TaskID)
	if err != nil {
		return err
	}
	req.Chat = resumeCursor(latest, artifacts, req.Key)
	req.TaskSession = &models.TaskSession{Role: role, Mode: "work", Settings: &c.Settings, ExpectedStage: c.Settings.WorkingStage}
	req.Conductor = &models.ConductorRun{Independent: true, Phase: "working", Settings: c.Settings, TaskAgent: agent, SourceBody: task.Body}
	return s.Repo.Advance(t, task, task.Body, c.Settings.WorkingStage, "queued", "Task session queued", req, c.Settings)
}
