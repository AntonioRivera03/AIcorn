package models

import "encoding/json"

// AIRunRequest is an immutable snapshot resolved before enqueueing. Manual runs
// do not control task stages; only Conductor carries an explicit stage contract.
type AgentSnapshot struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

// ChatTurn is resolved only by the server from this ticket's prior run artifacts.
// Client JSON never supplies a session ID or working directory.
type ChatTurn struct {
	ClientKey   string `json:"clientKey"`
	PreviousJob int    `json:"previousJob"`
	SessionID   string `json:"sessionId,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	Branch      string `json:"branch,omitempty"`
	BaseCommit  string `json:"baseCommit,omitempty"`
}

type ProjectChatTurn struct {
	ConversationID int             `json:"conversationId"`
	TurnID         int             `json:"turnId"`
	SessionID      string          `json:"sessionId,omitempty"`
	Context        json.RawMessage `json:"context"`
	TaskIDs        []int           `json:"taskIds"`
}

type AIRunRequest struct {
	AgentModels    map[string]string `json:"agentModels,omitempty"`
	ProjectChat    *ProjectChatTurn  `json:"projectChat,omitempty"`
	Chat           *ChatTurn         `json:"chat,omitempty"`
	Engine         string            `json:"engine"`
	AgentID        int               `json:"agentId,omitempty"`
	Conductor      *ConductorRun     `json:"conductor,omitempty"`
	Key            string            `json:"key"`
	Intent         string            `json:"intent"`
	Instruction    string            `json:"instruction"`
	TaskName       string            `json:"taskName"`
	TaskBody       string            `json:"taskBody"`
	PresetName     string            `json:"presetName,omitempty"`
	SystemPrompt   string            `json:"systemPrompt,omitempty"`
	Model          string            `json:"model"`
	Executable     string            `json:"executable"`
	EngineVersion  string            `json:"engineVersion"`
	RepoPath       string            `json:"repoPath,omitempty"`
	ProjectID      int               `json:"projectId"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type AISettings struct {
	Model          string `json:"model"`
	Executable     string `json:"executable"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type AIRunArtifacts struct {
	TurnDiff   string   `json:"turnDiff,omitempty"`
	TurnFiles  []string `json:"turnFiles,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	SessionID  string   `json:"sessionId,omitempty"`
	TurnID     string   `json:"turnId,omitempty"`
	Workspace  string   `json:"workspace,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	BaseCommit string   `json:"baseCommit,omitempty"`
	Diff       string   `json:"diff,omitempty"`
	Files      []string `json:"files,omitempty"`
	Warning    string   `json:"warning,omitempty"`
}
