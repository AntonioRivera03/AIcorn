package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	PersonaHarnessOpencode PersonaHarness = "opencode"

	PersonaAgentCodeAnalysis      PersonaAgent = "code-analysis"
	PersonaAgentCodeImplementation PersonaAgent = "code-implementation"
	PersonaAgentGeneralJunior     PersonaAgent = "general-junior"
	PersonaAgentGeneralSenior     PersonaAgent = "general-senior"
	PersonaAgentOxCodingAgent     PersonaAgent = "ox-coding-agent"
	PersonaAgentResearch          PersonaAgent = "research"
	PersonaAgentReviewer          PersonaAgent = "reviewer"
	PersonaAgentSummarizer        PersonaAgent = "summarizer"
	PersonaAgentSynthesis         PersonaAgent = "synthesis"
	PersonaAgentTestIntegration   PersonaAgent = "test-integration"
	PersonaAgentWorker            PersonaAgent = "worker"

	PersonaModelSonnet PersonaModel = "sonnet"
	PersonaModelOpus   PersonaModel = "opus"
	PersonaModelHaiku  PersonaModel = "haiku"

	// New opencode-go catalog — verified via `opencode models opencode-go` (23 models).
	// Kept for backwards compat: sonnet/opus/haiku still validate via IsValidPersonaModel.
	PersonaModelDeepseekV4Flash            PersonaModel = "opencode-go/deepseek-v4-flash"
	PersonaModelDeepseekV4FlashVisionExp   PersonaModel = "opencode-go/deepseek-v4-flash-vision-exp"
	PersonaModelDeepseekV4Pro              PersonaModel = "opencode-go/deepseek-v4-pro"
	PersonaModelGLM51                      PersonaModel = "opencode-go/glm-5.1"
	PersonaModelGLM52                      PersonaModel = "opencode-go/glm-5.2"
	PersonaModelGLM53                      PersonaModel = "opencode-go/glm-5.3"
	PersonaModelGLM53Flash                 PersonaModel = "opencode-go/glm-5.3-flash"
	PersonaModelGPT56Luna                  PersonaModel = "opencode-go/gpt-5.6-luna"
	PersonaModelGrok46                     PersonaModel = "opencode-go/grok-4.6"
	PersonaModelHy3                        PersonaModel = "opencode-go/hy3"
	PersonaModelKimiK26                    PersonaModel = "opencode-go/kimi-k2.6"
	PersonaModelKimiK27Code                PersonaModel = "opencode-go/kimi-k2.7-code"
	PersonaModelKimiK3                     PersonaModel = "opencode-go/kimi-k3"
	PersonaModelLongcat20                  PersonaModel = "opencode-go/longcat-2.0"
	PersonaModelMimoV25                    PersonaModel = "opencode-go/mimo-v2.5"
	PersonaModelMimoV25Pro                 PersonaModel = "opencode-go/mimo-v2.5-pro"
	PersonaModelMinimaxM27                 PersonaModel = "opencode-go/minimax-m2.7"
	PersonaModelMinimaxM3                  PersonaModel = "opencode-go/minimax-m3"
	PersonaModelMuseSpark12Contributor     PersonaModel = "opencode-go/muse-spark-1.2-contributor"
	PersonaModelQwen36Plus                 PersonaModel = "opencode-go/qwen3.6-plus"
	PersonaModelQwen37Max                  PersonaModel = "opencode-go/qwen3.7-max"
	PersonaModelQwen37Plus                 PersonaModel = "opencode-go/qwen3.7-plus"
	PersonaModelQwen38Max                  PersonaModel = "opencode-go/qwen3.8-max"
)

type PersonaHarness string

type PersonaModel string

type PersonaAgent string

var PersonaHarnesses = [...]PersonaHarness{
	PersonaHarnessOpencode,
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

// PersonaModels is the curated opencode-go catalog (23 models) as listed by
// `opencode models opencode-go`. Exact output (2026-08-26):
// opencode-go/deepseek-v4-flash
// opencode-go/deepseek-v4-flash-vision-exp
// opencode-go/deepseek-v4-pro
// opencode-go/glm-5.1
// opencode-go/glm-5.2
// opencode-go/glm-5.3
// opencode-go/glm-5.3-flash
// opencode-go/gpt-5.6-luna
// opencode-go/grok-4.6
// opencode-go/hy3
// opencode-go/kimi-k2.6
// opencode-go/kimi-k2.7-code
// opencode-go/kimi-k3
// opencode-go/longcat-2.0
// opencode-go/mimo-v2.5
// opencode-go/mimo-v2.5-pro
// opencode-go/minimax-m2.7
// opencode-go/minimax-m3
// opencode-go/muse-spark-1.2-contributor
// opencode-go/qwen3.6-plus
// opencode-go/qwen3.7-max
// opencode-go/qwen3.7-plus
// opencode-go/qwen3.8-max
// No migration needed: persona.harness and persona.model are plain TEXT columns
// without CHECK constraints, so adding values is validation-only.
var PersonaModels = [...]PersonaModel{
	PersonaModelDeepseekV4Flash,
	PersonaModelDeepseekV4FlashVisionExp,
	PersonaModelDeepseekV4Pro,
	PersonaModelGLM51,
	PersonaModelGLM52,
	PersonaModelGLM53,
	PersonaModelGLM53Flash,
	PersonaModelGPT56Luna,
	PersonaModelGrok46,
	PersonaModelHy3,
	PersonaModelKimiK26,
	PersonaModelKimiK27Code,
	PersonaModelKimiK3,
	PersonaModelLongcat20,
	PersonaModelMimoV25,
	PersonaModelMimoV25Pro,
	PersonaModelMinimaxM27,
	PersonaModelMinimaxM3,
	PersonaModelMuseSpark12Contributor,
	PersonaModelQwen36Plus,
	PersonaModelQwen37Max,
	PersonaModelQwen37Plus,
	PersonaModelQwen38Max,
}

type Persona struct {
	ID           int
	Name         string
	SystemPrompt string
	Harness      PersonaHarness
	Model        PersonaModel
	Agent        PersonaAgent
	AllowedTools []string
	TimeCreated  *time.Time
	TimeModified *time.Time
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
	case PersonaHarnessOpencode:
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
	switch model {
	case PersonaModelSonnet, PersonaModelOpus, PersonaModelHaiku,
		PersonaModelDeepseekV4Flash, PersonaModelDeepseekV4FlashVisionExp, PersonaModelDeepseekV4Pro,
		PersonaModelGLM51, PersonaModelGLM52, PersonaModelGLM53, PersonaModelGLM53Flash,
		PersonaModelGPT56Luna, PersonaModelGrok46, PersonaModelHy3,
		PersonaModelKimiK26, PersonaModelKimiK27Code, PersonaModelKimiK3,
		PersonaModelLongcat20, PersonaModelMimoV25, PersonaModelMimoV25Pro,
		PersonaModelMinimaxM27, PersonaModelMinimaxM3, PersonaModelMuseSpark12Contributor,
		PersonaModelQwen36Plus, PersonaModelQwen37Max, PersonaModelQwen37Plus, PersonaModelQwen38Max:
		return true
	default:
		return false
	}
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
