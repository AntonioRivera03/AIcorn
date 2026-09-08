package models

// AIRunRequest is an immutable snapshot resolved before enqueueing. Manual runs
// do not control task stages; only Conductor carries an explicit stage contract.
type AgentSnapshot struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type AIRunRequest struct {
	Engine         string        `json:"engine"`
	AgentID        int           `json:"agentId,omitempty"`
	Conductor      *ConductorRun `json:"conductor,omitempty"`
	Key            string        `json:"key"`
	Intent         string        `json:"intent"`
	Instruction    string        `json:"instruction"`
	TaskName       string        `json:"taskName"`
	TaskBody       string        `json:"taskBody"`
	PresetName     string        `json:"presetName,omitempty"`
	SystemPrompt   string        `json:"systemPrompt,omitempty"`
	Model          string        `json:"model"`
	Executable     string        `json:"executable"`
	EngineVersion  string        `json:"engineVersion"`
	RepoPath       string        `json:"repoPath,omitempty"`
	ProjectID      int           `json:"projectId"`
	TimeoutSeconds int           `json:"timeoutSeconds"`
}

type AISettings struct {
	Model          string `json:"model"`
	Executable     string `json:"executable"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type AIRunArtifacts struct {
	Workspace  string   `json:"workspace,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	BaseCommit string   `json:"baseCommit,omitempty"`
	Diff       string   `json:"diff,omitempty"`
	Files      []string `json:"files,omitempty"`
	Warning    string   `json:"warning,omitempty"`
}
