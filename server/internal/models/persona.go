package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
)

const (
	PersonaHarnessCodex      PersonaHarness = "codex"
	PersonaHarnessClaudeCode PersonaHarness = "claude-code"

	PersonaAgentCodeAnalysis       PersonaAgent = "code-analysis"
	PersonaAgentCodeImplementation PersonaAgent = "code-implementation"
	PersonaAgentGeneralJunior      PersonaAgent = "general-junior"
	PersonaAgentGeneralSenior      PersonaAgent = "general-senior"
	PersonaAgentOxCodingAgent      PersonaAgent = "ox-coding-agent"
	PersonaAgentResearch           PersonaAgent = "research"
	PersonaAgentReviewer           PersonaAgent = "reviewer"
	PersonaAgentSummarizer         PersonaAgent = "summarizer"
	PersonaAgentSynthesis          PersonaAgent = "synthesis"
	PersonaAgentTestIntegration    PersonaAgent = "test-integration"
	PersonaAgentWorker             PersonaAgent = "worker"

	PersonaModelDefault PersonaModel = "gpt-5.6-sol"
	PersonaModelAstra   PersonaModel = "gpt-6-astra"
)

type PersonaHarness string

type PersonaModel string

type PersonaAgent string

var PersonaHarnesses = [...]PersonaHarness{
	PersonaHarnessCodex,
	PersonaHarnessClaudeCode,
}

var PersonaAgents = [...]PersonaAgent{
	PersonaAgentCodeAnalysis,
	PersonaAgentCodeImplementation,
	PersonaAgentGeneralJunior,
	PersonaAgentGeneralSenior,
	PersonaAgentOxCodingAgent,
	PersonaAgentResearch,
	PersonaAgentReviewer,
	PersonaAgentSummarizer,
	PersonaAgentSynthesis,
	PersonaAgentTestIntegration,
	PersonaAgentWorker,
}

var openAIModel = regexp.MustCompile(`^(gpt-[0-9][a-z0-9.-]*|o[0-9][a-z0-9.-]*|codex-mini-latest)$`)
var claudeModel = regexp.MustCompile(`^claude-[a-z0-9][a-z0-9.-]*$`)

func IsOpenAIModel(model string) bool { return len(model) <= 100 && openAIModel.MatchString(model) }
func IsClaudeModel(model string) bool { return len(model) <= 100 && claudeModel.MatchString(model) }

// IsHarnessModel checks a model ID's family rather than a fixed list, because
// harnesses publish new models without an Aycorn release.
func IsHarnessModel(harness PersonaHarness, model string) bool {
	switch harness {
	case PersonaHarnessCodex:
		return IsOpenAIModel(model)
	case PersonaHarnessClaudeCode:
		return IsClaudeModel(model)
	default:
		return false
	}
}

type Persona struct {
	BuiltinRole string
	// TaskAgent marks the bundled agents users can pick and Conductor can
	// dispatch (Coder, Reviewer, Research). Internal and retired agents are false.
	TaskAgent       bool
	Description     string
	Instructions    string
	InstructionPath string
	Skills          []AgentSkill
	ID              int
	Name            string
	SystemPrompt    string
	Harness         PersonaHarness
	Model           PersonaModel
	Agent           PersonaAgent
	AllowedTools    []string
	TimeCreated     *time.Time
	TimeModified    *time.Time
}

type AgentSkill struct{ Name, Path, Content string }

// Bundled definitions override legacy editable fields without destroying them
// in storage. Every reader sees the same authoritative role and Markdown.
func (p *Persona) ApplyBuiltin() {
	if d, ok := fleet.Lookup(p.BuiltinRole); ok {
		p.Name, p.Description, p.Instructions = d.Name, d.Description, d.Instructions()
		p.TaskAgent = !d.Internal
		p.Harness = PersonaHarnessCodex
		p.InstructionPath = "server/internal/harness/fleet/" + d.Role + "-instructions.md"
		p.Skills = []AgentSkill{{Name: "aycorn-workflow", Path: "server/internal/harness/fleet/" + fleet.WorkflowPath, Content: fleet.Workflow()}}
		body, _ := json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": p.Instructions}}}})
		p.SystemPrompt = string(body)
	}
}

type PersonaSummary struct {
	ID      int
	Name    string
	Harness PersonaHarness
	Model   PersonaModel
	Agent   PersonaAgent
}

type StagePersona struct {
	StageID   int
	PersonaID int
}

func IsValidPersonaHarness(harness PersonaHarness) bool {
	switch harness {
	case PersonaHarnessCodex, PersonaHarnessClaudeCode:
		return true
	default:
		return false
	}
}

func IsValidPersonaAgent(agent PersonaAgent) bool {
	if agent == "" {
		return true
	}
	switch agent {
	case PersonaAgentCodeAnalysis, PersonaAgentCodeImplementation, PersonaAgentGeneralJunior,
		PersonaAgentGeneralSenior, PersonaAgentOxCodingAgent, PersonaAgentResearch,
		PersonaAgentReviewer, PersonaAgentSummarizer, PersonaAgentSynthesis,
		PersonaAgentTestIntegration, PersonaAgentWorker:
		return true
	default:
		return false
	}
}

func IsValidPersonaModel(model PersonaModel) bool {
	return IsOpenAIModel(string(model)) || IsClaudeModel(string(model))
}

func ParseAllowedTools(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return []string{}, nil
	}

	var tools []string
	if err := json.Unmarshal([]byte(trimmed), &tools); err != nil {
		return nil, fmt.Errorf("decode allowed tools: %w", err)
	}
	if tools == nil {
		return []string{}, nil
	}
	return tools, nil
}

func EncodeAllowedTools(tools []string) (string, error) {
	if len(tools) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("encode allowed tools: %w", err)
	}
	return string(encoded), nil
}
