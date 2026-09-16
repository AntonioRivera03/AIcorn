package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/worktree"
	"os"
	"os/exec"
	"strings"
)

var ErrInvalidAIRun = errors.New("invalid AI request")
var ErrAISetup = errors.New("AI setup needs attention")

type AIRunInput struct {
	Engine        string                `json:"engine,omitempty"`
	Agent         *models.AgentSnapshot `json:"-"` // Frozen custom agent for a Conductor cycle.
	Intent        string                `json:"intent"`
	Instruction   string                `json:"instruction"`
	PresetID      int                   `json:"presetId"`
	UseRepository bool                  `json:"useRepository"`
}
type AIService struct {
	Jobs          *repos.AgentJobRepo
	Tasks         *repos.TaskRepo
	Projects      *repos.ProjectRepo
	Presets       *repos.PersonaRepo
	Converter     *markdown.Converter
	MCPExecutable string
	Probe         func(context.Context, string) harness.EngineHealth
}

func (s *AIService) Health(ctx context.Context, executable string) harness.EngineHealth {
	if s.Probe != nil {
		return s.Probe(ctx, executable)
	}
	health := harness.CheckEngine(ctx, executable)
	if info, err := os.Stat(s.MCPExecutable); err != nil || info.IsDir() {
		health.Ready = false
		health.Error = "Aycorn's MCP executable is missing. Run make build-mcp or install aycorn-mcp beside aycorn."
	}
	if _, err := exec.LookPath("node"); err != nil {
		health.Ready = false
		health.Error = "Node.js is required to convert task descriptions for AI. Install Node.js and restart Aycorn."
	}
	return health
}
func (s *AIService) UpdateSettings(settings models.AISettings) error {
	settings.Model = strings.TrimSpace(settings.Model)
	settings.Executable = strings.TrimSpace(settings.Executable)
	if settings.Model != "" && !models.IsOpenAIModel(settings.Model) {
		return fmt.Errorf("%w: choose an OpenAI model ID (for example gpt-5.6-sol)", ErrInvalidAIRun)
	}
	if settings.TimeoutSeconds < 10 || settings.TimeoutSeconds > 1800 {
		return fmt.Errorf("%w: timeout must be 10–1800 seconds", ErrInvalidAIRun)
	}
	return s.Jobs.UpdateAISettings(settings)
}
func (s *AIService) Start(ctx context.Context, taskID int, in AIRunInput) (*models.AgentJob, error) {
	req, err := s.Prepare(ctx, taskID, in)
	if err != nil {
		return nil, err
	}
	return s.Jobs.EnqueueAI(taskID, in.PresetID, *req)
}

// Prepare resolves an immutable request without enqueueing it. Conductor uses
// this so its state transition and queue insertion can share one transaction.
func (s *AIService) Prepare(ctx context.Context, taskID int, in AIRunInput) (*models.AIRunRequest, error) {
	if in.Engine != "" && in.Engine != "codex" {
		return nil, fmt.Errorf("%w: %s is unavailable; select Codex", ErrInvalidAIRun, in.Engine)
	}
	if in.Intent == "" {
		in.Intent = "ask"
	}
	switch in.Intent {
	case "ask", "plan", "implement", "review":
	default:
		return nil, fmt.Errorf("%w: unknown intent", ErrInvalidAIRun)
	}
	in.Instruction = strings.TrimSpace(in.Instruction)
	if len(in.Instruction) > 32000 || in.PresetID < 0 {
		return nil, fmt.Errorf("%w: instruction too long or invalid preset", ErrInvalidAIRun)
	}
	task, err := s.Tasks.FindOneWithProject(taskID)
	if err != nil {
		return nil, err
	}
	return s.PrepareSnapshot(ctx, task, in)
}

// PrepareSnapshot resolves a template before its task is inserted, allowing a
// scheduler to commit the new task and its queued run in one transaction.
func (s *AIService) PrepareSnapshot(ctx context.Context, task *models.TaskWithProject, in AIRunInput) (*models.AIRunRequest, error) {
	if in.Engine != "" && in.Engine != "codex" {
		return nil, ErrInvalidAIRun
	}
	if in.Intent == "" {
		in.Intent = "ask"
	}
	switch in.Intent {
	case "ask", "plan", "implement", "review":
	default:
		return nil, ErrInvalidAIRun
	}
	in.Instruction = strings.TrimSpace(in.Instruction)
	if task == nil || len(in.Instruction) > 32000 || in.PresetID < 0 {
		return nil, ErrInvalidAIRun
	}
	settings, err := s.Jobs.AISettings()
	if err != nil {
		return nil, err
	}
	agent := in.Agent
	if agent == nil && in.PresetID > 0 {
		agent, err = s.ResolveAgent(ctx, in.PresetID)
		if err != nil {
			return nil, err
		}
	}
	if agent != nil {
		settings.Model = agent.Model
	}
	if !models.IsOpenAIModel(settings.Model) {
		return nil, fmt.Errorf("%w: choose an OpenAI model in AI settings", ErrAISetup)
	}
	health := s.Health(ctx, settings.Executable)
	if !health.Ready {
		return nil, fmt.Errorf("%w: %s", ErrAISetup, health.Error)
	}
	req := models.AIRunRequest{Engine: "codex", Intent: in.Intent, Instruction: in.Instruction, TaskName: task.Name, Model: settings.Model, Executable: health.Executable, EngineVersion: health.Version, ProjectID: task.ProjectID, TimeoutSeconds: settings.TimeoutSeconds}

	if agent != nil {
		req.AgentID = agent.ID
		req.PresetName = agent.Name
		req.SystemPrompt = agent.Instructions
	}
	converted, err := s.Converter.ToMarkdown(ctx, []string{task.Body})
	if err != nil {
		return nil, err
	}
	req.TaskBody = converted[0]
	if in.UseRepository || in.Intent == "implement" || in.Intent == "review" {
		project, err := s.Projects.FindOne(task.ProjectID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(project.RepoPath) == "" {
			return nil, ErrRepoPathMissing
		}
		root, err := worktree.RepoRootFromDir(ctx, project.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRepoInvalid, err)
		}
		req.RepoPath = root
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	req.Key = hex.EncodeToString(key)
	return &req, nil
}

func (s *AIService) Settings() (models.AISettings, error) { return s.Jobs.AISettings() }
func (s *AIService) Cancel(id int) (bool, error) {
	if _, err := s.Jobs.FindOne(id); err != nil {
		return false, err
	}
	return s.Jobs.CancelAI(id)
}

// ResolveAgent snapshots editable instructions and model once per planning cycle.
func (s *AIService) ResolveAgent(ctx context.Context, id int) (*models.AgentSnapshot, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: select a custom agent from the AI page", ErrAISetup)
	}
	p, err := s.Presets.FindOne(id)
	if err != nil {
		return nil, fmt.Errorf("%w: selected agent is missing; choose another agent", ErrAISetup)
	}
	if p.Harness != models.PersonaHarnessCodex || !models.IsValidPersonaModel(p.Model) {
		return nil, fmt.Errorf("%w: selected agent must use Codex and an OpenAI model", ErrAISetup)
	}
	prompts, err := s.Converter.ToMarkdown(ctx, []string{p.SystemPrompt})
	if err != nil {
		return nil, err
	}
	return &models.AgentSnapshot{ID: p.ID, Name: p.Name, Model: string(p.Model), Instructions: prompts[0]}, nil
}
