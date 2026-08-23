package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	PersonaHarnessClaudeCode PersonaHarness = "claude-code"

	PersonaModelSonnet PersonaModel = "sonnet"
	PersonaModelOpus   PersonaModel = "opus"
	PersonaModelHaiku  PersonaModel = "haiku"
)

type PersonaHarness string

type PersonaModel string

var PersonaHarnesses = [...]PersonaHarness{
	PersonaHarnessClaudeCode,
}

var PersonaModels = [...]PersonaModel{
	PersonaModelSonnet,
	PersonaModelOpus,
	PersonaModelHaiku,
}

type Persona struct {
	ID           int
	Name         string
	SystemPrompt string
	Harness      PersonaHarness
	Model        PersonaModel
	AllowedTools []string
	TimeCreated  *time.Time
	TimeModified *time.Time
}

type PersonaSummary struct {
	ID      int
	Name    string
	Harness PersonaHarness
	Model   PersonaModel
}

type StagePersona struct {
	StageID   int
	PersonaID int
}

func IsValidPersonaHarness(harness PersonaHarness) bool {
	switch harness {
	case PersonaHarnessClaudeCode:
		return true
	default:
		return false
	}
}

func IsValidPersonaModel(model PersonaModel) bool {
	switch model {
	case PersonaModelSonnet, PersonaModelOpus, PersonaModelHaiku:
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
