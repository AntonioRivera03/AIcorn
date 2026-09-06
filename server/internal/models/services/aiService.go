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
	"regexp"
	"strings"
)

var ErrInvalidAIRun = errors.New("invalid AI request")
var ErrAISetup = errors.New("AI setup needs attention")
var modelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*$`)

type AIRunInput struct {
	Intent        string `json:"intent"`
	Instruction   string `json:"instruction"`
	PresetID      int    `json:"presetId"`
	UseRepository bool   `json:"useRepository"`
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
	return health
}
func (s *AIService) UpdateSettings(settings models.AISettings) error {
	settings.Model = strings.TrimSpace(settings.Model)
	settings.Executable = strings.TrimSpace(settings.Executable)
	if settings.Model != "" && (!modelID.MatchString(settings.Model) || !strings.Contains(settings.Model, "/")) {
		return fmt.Errorf("%w: model must use provider/model format", ErrInvalidAIRun)
	}
	if settings.TimeoutSeconds < 10 || settings.TimeoutSeconds > 1800 {
		return fmt.Errorf("%w: timeout must be 10–1800 seconds", ErrInvalidAIRun)
	}
	return s.Jobs.UpdateAISettings(settings)
}
func (s *AIService) Start(ctx context.Context, taskID int, in AIRunInput) (*models.AgentJob, error) {
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
	settings, err := s.Jobs.AISettings()
	if err != nil {
		return nil, err
	}
	if !modelID.MatchString(settings.Model) || !strings.Contains(settings.Model, "/") {
		return nil, fmt.Errorf("%w: choose a provider/model in AI settings", ErrAISetup)
	}
	health := s.Health(ctx, settings.Executable)
	if !health.Ready {
		return nil, fmt.Errorf("%w: %s", ErrAISetup, health.Error)
	}
	req := models.AIRunRequest{Intent: in.Intent, Instruction: in.Instruction, TaskName: task.Name, Model: settings.Model, Executable: health.Executable, EngineVersion: health.Version, ProjectID: task.ProjectID, TimeoutSeconds: settings.TimeoutSeconds}
	bodies := []string{task.Body}
	if in.PresetID > 0 {
		preset, err := s.Presets.FindOne(in.PresetID)
		if err != nil {
			return nil, err
		}
		req.PresetName = preset.Name
		bodies = append(bodies, preset.SystemPrompt)
	}
	converted, err := s.Converter.ToMarkdown(ctx, bodies)
	if err != nil {
		return nil, err
	}
	req.TaskBody = converted[0]
	if len(converted) > 1 {
		req.SystemPrompt = converted[1]
	}
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
	return s.Jobs.EnqueueAI(taskID, in.PresetID, req)
}

func (s *AIService) Settings() (models.AISettings, error) { return s.Jobs.AISettings() }
func (s *AIService) Cancel(id int) (bool, error) {
	if _, err := s.Jobs.FindOne(id); err != nil {
		return false, err
	}
	return s.Jobs.CancelAI(id)
}
