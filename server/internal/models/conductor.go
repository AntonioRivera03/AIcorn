package models

// Stage roles are project configuration, independent of workflow stage types.
// Completion is always a handoff to a human, so it cannot use a done stage.
type ConductorSettings struct {
	Enabled          bool   `json:"enabled"`
	PlanningStage    int    `json:"planningStage"`
	WorkingStage     int    `json:"workingStage"`
	CompletionStage  int    `json:"completionStage"`
	PlanningPrompt   string `json:"planningPrompt"`
	WorkingPrompt    string `json:"workingPrompt"`
	CompletionPrompt string `json:"completionPrompt"`
	ConductorAgentID int    `json:"conductorAgentId"`
	TaskAgentID      int    `json:"taskAgentId"`
	UseRepository    bool   `json:"useRepository"`
}

func DefaultConductorSettings() ConductorSettings {
	return ConductorSettings{
		PlanningPrompt:   "Inspect the tasks given to Conductor. Start eligible tasks with the appropriate independent agent. Defer tasks with unresolved dependencies or essential missing context, giving a specific reason.",
		WorkingPrompt:    "Carry out the task and its acceptance criteria. Follow the project instructions. Explain any blockers and distinguish verified results from assumptions.",
		CompletionPrompt: "Write a handoff for human review: what changed, acceptance criteria addressed, validation performed, limitations, and what the reviewer should check. Never claim unperformed tests passed.",
		UseRepository:    true,
	}
}

type ConductorTask struct {
	SelectionKey  string `json:"-"`
	TaskID        int    `json:"taskId"`
	ProjectID     int    `json:"projectId"`
	State         string `json:"state"`
	JobID         int    `json:"jobId"`
	ExpectedStage int    `json:"expectedStage"`
	Message       string `json:"message"`
	UpdatedAt     string `json:"updatedAt"`
}

// ConductorRun freezes the stage contract and model choices for one task cycle.
type ConductorRun struct {
	Independent    bool              `json:"independent,omitempty"` // Scheduled/manual Jobs ignore the board toggle.
	ConductorAgent *AgentSnapshot    `json:"conductorAgent,omitempty"`
	TaskAgent      *AgentSnapshot    `json:"taskAgent,omitempty"`
	Phase          string            `json:"phase"`
	Settings       ConductorSettings `json:"settings"`
	SourceBody     string            `json:"sourceBody"`
}

type ConductorDecision struct {
	Ready          *bool  `json:"ready"`
	Context        string `json:"context"`
	MissingContext string `json:"missingContext"`
}
