package models

// Stage roles are project configuration, independent of workflow stage types.
// Completion is always a handoff to a human, so it cannot use a done stage.
//
// WorkingPrompt and CompletionPrompt are built into Aycorn, not project
// settings: every read replaces them with the built-in text (see
// UseBuiltinInstructions). They stay on the struct because a run's snapshot
// carries them, and a Job appends its own instructions to WorkingPrompt.
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

const (
	conductorWorkingInstructions = "Carry out the task and its acceptance criteria. Follow the project instructions. Explain any blockers and distinguish verified results from assumptions."
	conductorHandoffInstructions = "Write a handoff for human review: what changed, acceptance criteria addressed, validation performed, limitations, and what the reviewer should check. Never claim unperformed tests passed."
)

func DefaultConductorSettings() ConductorSettings {
	// PlanningPrompt is the project's own, optional guidance for choosing
	// tasks; how to choose them is already in Conductor's fixed instructions.
	s := ConductorSettings{UseRepository: true}
	s.UseBuiltinInstructions()
	return s
}

// UseBuiltinInstructions sets the agent's working and handoff instructions to
// Aycorn's own, replacing any a project saved when they were editable.
func (s *ConductorSettings) UseBuiltinInstructions() {
	s.WorkingPrompt = conductorWorkingInstructions
	s.CompletionPrompt = conductorHandoffInstructions
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
