package models

import "time"

const (
	AgentJobStatusPending     = "pending"
	AgentJobStatusClaimed     = "claimed"
	AgentJobStatusRunning     = "running"
	AgentJobStatusCompleted   = "completed"
	AgentJobStatusFailed      = "failed"
	AgentJobStatusCanceled    = "canceled"
	AgentJobStatusInterrupted = "interrupted"
)

// AgentJob is the durable queue entry for one persona execution.
// Status is plain TEXT (not a CHECK enum) so new statuses can be added
// without a full-table-recreate migration.
type AgentJob struct {
	Request    *AIRunRequest `json:"request,omitempty"`
	Progress   string        `json:"progress"`
	ID         int           `json:"id"`
	Task       int           `json:"task"`
	Persona    int           `json:"persona"`
	Status     string        `json:"status"`
	FromStage  *int          `json:"fromStage"`
	ToStage    *int          `json:"toStage"`
	ClaimedAt  *time.Time    `json:"claimedAt"`
	StartedAt  *time.Time    `json:"startedAt"`
	FinishedAt *time.Time    `json:"finishedAt"`
	Attempts   int           `json:"attempts"`
	Error      string        `json:"error"`
	CreatedAt  *time.Time    `json:"createdAt"`
}

// AgentJobsResponse is the payload for GET /api/agent-jobs/{taskId}.
// It bundles jobs for the task and all runs belonging to those jobs.
// Frontend can render job status/timestamps from Jobs and output/usage from Runs.
// Shape: { "jobs": [...AgentJob], "runs": [...AgentRun] }
type AgentJobsResponse struct {
	Jobs []AgentJob `json:"jobs"`
	Runs []AgentRun `json:"runs"`
}
