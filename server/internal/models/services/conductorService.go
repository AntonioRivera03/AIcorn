package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
)

type ConductorService struct {
	Repo *repos.ConductorRepo
	AI   *AIService
	Runs *repos.AgentRunRepo
}

type ConductorBoard struct {
	Settings           models.ConductorSettings `json:"settings"`
	Tasks              []models.ConductorTask   `json:"tasks"`
	ConfigurationError string                   `json:"configurationError,omitempty"`
}

func (s *ConductorService) Board(project int) (ConductorBoard, error) {
	c, _, err := s.Repo.Settings(project)
	if err != nil {
		return ConductorBoard{}, err
	}
	tasks, err := s.Repo.Tasks(project)
	b := ConductorBoard{Settings: c, Tasks: tasks}
	if configErr := s.Repo.ValidateStages(project, c); configErr != nil {
		b.ConfigurationError = configErr.Error()
	} else if c.ConductorAgentID == 0 || c.TaskAgentID == 0 {
		b.ConfigurationError = "Choose a Conductor agent and task agent from the AI page."
	} else {
		for _, id := range []int{c.ConductorAgentID, c.TaskAgentID} {
			p, err := s.AI.Presets.FindOne(id)
			if err != nil || p.Harness != models.PersonaHarnessCodex || !models.IsValidPersonaModel(p.Model) {
				b.ConfigurationError = "A selected agent is unavailable. Choose a Codex agent in project settings."
				break
			}
		}
	}
	return b, err
}

// Settings are field patches, auto-saved in place. Unknown keys are rejected.
func (s *ConductorService) UpdateSettings(ctx context.Context, project int, patch map[string]json.RawMessage) (models.ConductorSettings, error) {
	current, previous, err := s.Repo.Settings(project)
	if err != nil {
		return current, err
	}
	raw, _ := json.Marshal(current)
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return current, err
	}
	for key, value := range patch {
		if _, ok := fields[key]; !ok || string(value) == "null" {
			return current, fmt.Errorf("%w: invalid Conductor setting %s", ErrInvalidAIRun, key)
		}
		fields[key] = value
	}
	raw, err = json.Marshal(fields)
	if err != nil {
		return current, err
	}
	var next models.ConductorSettings
	if err = json.Unmarshal(raw, &next); err != nil {
		return current, fmt.Errorf("%w: %v", ErrInvalidAIRun, err)
	}
	if next.PlanningStage < 0 || next.WorkingStage < 0 || next.CompletionStage < 0 {
		return current, repos.ErrConductorConfig
	}
	for _, prompt := range []string{next.PlanningPrompt, next.WorkingPrompt, next.CompletionPrompt} {
		if len(prompt) > 16000 {
			return current, fmt.Errorf("%w: stage prompts must be under 16000 characters", ErrInvalidAIRun)
		}
	}
	if next.ConductorAgentID < 0 || next.TaskAgentID < 0 {
		return current, fmt.Errorf("%w: invalid agent", ErrInvalidAIRun)
	}
	for key, id := range map[string]int{"conductorAgentId": next.ConductorAgentID, "taskAgentId": next.TaskAgentID} {
		_, changed := patch[key]
		if id > 0 && (next.Enabled || changed) {
			if _, err := s.AI.ResolveAgent(ctx, id); err != nil {
				return current, err
			}
		}
	}
	if next.Enabled {
		if err = s.Repo.ValidateStages(project, next); err != nil {
			return current, err
		}
		settings, err := s.AI.Settings()
		if err != nil {
			return current, err
		}
		if next.ConductorAgentID == 0 || next.TaskAgentID == 0 {
			return current, fmt.Errorf("%w: select a Conductor agent and task agent", ErrAISetup)
		}
		if !current.Enabled {
			health := s.AI.Health(ctx, settings.Executable)
			if !health.Ready {
				return current, fmt.Errorf("%w: %s", ErrAISetup, health.Error)
			}
		}
		if next.UseRepository {
			p, err := s.AI.Projects.FindOne(project)
			if err != nil {
				return current, err
			}
			if strings.TrimSpace(p.RepoPath) == "" {
				return current, ErrRepoPathMissing
			}
			if _, err = worktree.RepoRootFromDir(ctx, p.RepoPath); err != nil {
				return current, fmt.Errorf("%w: %v", ErrRepoInvalid, err)
			}
		}
	}
	return next, s.Repo.SaveSettings(project, next, previous)
}

func (s *ConductorService) Manage(project int, ids []int, action string) (models.BulkResult, error) {
	if len(ids) == 0 || len(ids) > 500 {
		return models.BulkResult{}, fmt.Errorf("%w: select 1–500 tasks", ErrInvalidAIRun)
	}
	if action != "send" && action != "release" && action != "recheck" {
		return models.BulkResult{}, fmt.Errorf("%w: unknown Conductor action", ErrInvalidAIRun)
	}
	if _, _, err := s.Repo.Settings(project); err != nil {
		return models.BulkResult{}, err
	}
	return s.Repo.Manage(project, ids, action)
}

// Tick reconciles durable job results before scheduling new planning. A crash
// between finishing a run and this transaction cannot lose or duplicate a handoff.
func (s *ConductorService) Tick(ctx context.Context) error {
	if err := s.Repo.ReleaseReviewed(); err != nil {
		return err
	}
	tasks, err := s.Repo.Tasks(0)
	if err != nil {
		return err
	}
	scheduled := false
	for _, t := range tasks {
		if t.State == "waiting" && scheduled {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if t.State != "waiting" && t.State != "planning" && t.State != "queued" && t.State != "working" {
			continue
		}
		err = s.reconcile(ctx, t)
		if t.State == "waiting" && err == nil {
			scheduled = true
		}
		if err == nil || errors.Is(err, repos.ErrConductorPaused) || errors.Is(err, repos.ErrActiveAIRun) {
			continue
		}
		state := "failed"
		if errors.Is(err, repos.ErrConductorBlocked) {
			state = "needs_context"
		}
		if errors.Is(err, repos.ErrConductorConflict) || errors.Is(err, repos.ErrConductorConfig) {
			state = "held"
		}
		if t.JobID > 0 {
			if _, cancelErr := s.AI.Jobs.CancelAI(t.JobID); cancelErr != nil {
				return cancelErr
			}
		}
		if saveErr := s.Repo.SetState(t, state, err.Error()); saveErr != nil {
			return saveErr
		}
	}
	return nil
}

func (s *ConductorService) reconcile(ctx context.Context, t models.ConductorTask) error {
	task, err := s.AI.Tasks.FindOneWithProject(t.TaskID)
	if err != nil {
		return err
	}
	task.Body, err = s.AI.Tasks.RawTaskBody(t.TaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != t.ProjectID || task.Stage != t.ExpectedStage {
		if t.JobID > 0 {
			if _, err = s.AI.Jobs.CancelAI(t.JobID); err != nil {
				return err
			}
		}
		return fmt.Errorf("%w: task was moved manually; recheck it to resume", repos.ErrConductorConflict)
	}
	settings, _, err := s.Repo.Settings(t.ProjectID)
	if err != nil {
		return err
	}
	if t.State == "waiting" {
		if !settings.Enabled {
			return repos.ErrConductorPaused
		}
		if err = s.Repo.ValidateStages(t.ProjectID, settings); err != nil {
			return err
		}
		req, err := s.AI.Prepare(ctx, t.TaskID, AIRunInput{Intent: "plan", UseRepository: settings.UseRepository, PresetID: settings.ConductorAgentID, Instruction: settings.PlanningPrompt})
		if err != nil {
			return err
		}
		taskAgent, err := s.AI.ResolveAgent(ctx, settings.TaskAgentID)
		if err != nil {
			return err
		}
		req.Conductor = &models.ConductorRun{Phase: "planning", Settings: settings, SourceBody: task.Body, TaskAgent: taskAgent, ConductorAgent: &models.AgentSnapshot{ID: req.AgentID, Name: req.PresetName, Model: req.Model, Instructions: req.SystemPrompt}}
		return s.Repo.Advance(t, task, task.Body, settings.PlanningStage, "planning", "Conductor is checking the task", req, settings)
	}
	if t.JobID == 0 {
		return errors.New("Conductor's run is missing; recheck the task")
	}
	job, err := s.AI.Jobs.FindOne(t.JobID)
	if err != nil {
		return err
	}
	if job.Request == nil || job.Request.Conductor == nil {
		return errors.New("Conductor's run contract is missing")
	}
	contract := job.Request.Conductor
	if err = s.Repo.ValidateStages(t.ProjectID, contract.Settings); err != nil {
		if _, cancelErr := s.AI.Jobs.CancelAI(job.ID); cancelErr != nil {
			return cancelErr
		}
		return err
	}
	switch job.Status {
	case "pending", "claimed", "running", "canceling":
		return nil
	case "completed":
	default:
		return fmt.Errorf("AI run %s: %s. Recheck when ready", job.Status, job.Error)
	}
	runs, err := s.Runs.ListByJob(job.ID)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return errors.New("The completed run has no result")
	}
	output := runs[len(runs)-1].Output
	if contract.Phase == "planning" {
		if task.Body != contract.SourceBody || task.Name != job.Request.TaskName {
			return fmt.Errorf("%w: task content changed during planning; recheck to use the new context", repos.ErrConductorConflict)
		}
		decision, err := ParseConductorDecision(output)
		if err != nil {
			return err
		}
		notes := decision.Context
		state, message := "needs_context", decision.MissingContext
		var req *models.AIRunRequest
		if *decision.Ready {
			if !settings.Enabled {
				return repos.ErrConductorPaused
			}
			intent := "ask"
			if contract.Settings.UseRepository {
				intent = "implement"
			}
			if contract.TaskAgent == nil {
				return errors.New("Task agent snapshot is missing; recheck the task")
			}
			rootAgent := contract.ConductorAgent
			if rootAgent == nil {
				rootAgent = contract.TaskAgent
			} // existing queued cycles
			req, err = s.AI.Prepare(ctx, t.TaskID, AIRunInput{Intent: intent, Agent: rootAgent, Instruction: contract.Settings.WorkingPrompt + "\n\nHuman review handoff requirements:\n" + contract.Settings.CompletionPrompt})
			if err != nil {
				return err
			}
			state, message = "queued", "Validated; waiting for an agent"
		} else {
			notes += "\n\n### Missing context\n" + decision.MissingContext
		}
		body, err := s.appendBody(ctx, task.Body, fmt.Sprintf("## Conductor planning · run %d\n\n%s", job.ID, notes))
		if err != nil {
			return err
		}
		if req != nil {
			converted, err := s.AI.Converter.ToMarkdown(ctx, []string{body})
			if err != nil {
				return err
			}
			req.TaskBody = converted[0]
			req.Conductor = &models.ConductorRun{Phase: "working", Settings: contract.Settings, SourceBody: body, ConductorAgent: contract.ConductorAgent, TaskAgent: contract.TaskAgent}
		}
		return s.Repo.Advance(t, task, body, task.Stage, state, message, req, contract.Settings)
	}
	var result ConductorWorkResult
	if err = decodeConductorJSON(output, &result); err != nil {
		return err
	}
	if result.Completed == nil || strings.TrimSpace(result.Summary) == "" || (!*result.Completed && strings.TrimSpace(result.Blocker) == "") {
		return errors.New("Agent returned an incomplete handoff; inspect the run and recheck")
	}
	state, message, stage := "completed", "Ready for human review", contract.Settings.CompletionStage
	notes := result.Summary
	if !*result.Completed {
		state, message, stage = "needs_context", result.Blocker, task.Stage
		notes += "\n\n### Blocker\n" + result.Blocker
	}
	body, err := s.appendBody(ctx, task.Body, fmt.Sprintf("## Conductor handoff · run %d\n\n%s", job.ID, notes))
	if err != nil {
		return err
	}
	return s.Repo.Advance(t, task, body, stage, state, message, nil, contract.Settings)
}

type ConductorWorkResult struct {
	Completed *bool  `json:"completed"`
	Summary   string `json:"summary"`
	Blocker   string `json:"blocker"`
}

func ParseConductorDecision(output string) (models.ConductorDecision, error) {
	var d models.ConductorDecision
	if err := decodeConductorJSON(output, &d); err != nil {
		return d, err
	}
	if d.Ready == nil || (*d.Ready && strings.TrimSpace(d.Context) == "") || (!*d.Ready && strings.TrimSpace(d.MissingContext) == "") {
		return d, errors.New("Conductor returned an incomplete planning decision; inspect the run and recheck")
	}

	return d, nil
}

func decodeConductorJSON(output string, dest any) error {
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "```json\n") && strings.HasSuffix(output, "```") {
		output = strings.TrimSuffix(strings.TrimPrefix(output, "```json\n"), "```")
	}
	d := json.NewDecoder(strings.NewReader(output))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return fmt.Errorf("Invalid Conductor result: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("Conductor returned more than one result")
	}
	return nil
}

// Append new Plate nodes without round-tripping the user's existing rich text.
func (s *ConductorService) appendBody(ctx context.Context, body, addition string) (string, error) {
	converted, err := s.AI.Converter.ToBody(ctx, []string{addition})
	if err != nil {
		return "", err
	}
	var existing, extra []json.RawMessage
	if strings.TrimSpace(body) != "" {
		if err = json.Unmarshal([]byte(body), &existing); err != nil {
			return "", err
		}
	}
	if err = json.Unmarshal([]byte(converted[0]), &extra); err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err = json.NewEncoder(&out).Encode(append(existing, extra...)); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func (s *ConductorService) BeginJob(job *models.AgentJob) (bool, error) {
	ok, err := s.Repo.BeginJob(job)
	if err != nil {
		// Cancel invalid claims instead of running with stale context or scope.
		if _, cancelErr := s.AI.Jobs.CancelAI(job.ID); cancelErr != nil {
			return false, cancelErr
		}
		previous := "planning"
		if job.Request.Conductor.Phase == "working" {
			previous = "queued"
		}
		state := "held"
		if errors.Is(err, repos.ErrConductorBlocked) {
			state = "needs_context"
		}
		message := err.Error()
		if errors.Is(err, sql.ErrNoRows) {
			message = "Task was moved or released before its agent started. Recheck to resume."
		}
		if saveErr := s.Repo.SetState(models.ConductorTask{TaskID: job.Task, JobID: job.ID, State: previous}, state, message); saveErr != nil {
			return false, saveErr
		}
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
	}
	return ok, err
}
