package models

import "time"

// AgentRun is one execution attempt for an AgentJob.
// Output is append-only markdown, UsageJson is an opaque JSON blob.
type AgentRun struct {
	ID        int        `json:"id"`
	Job       int        `json:"job"`
	Output    string     `json:"output"`
	Summary   string     `json:"summary"`
	ExitCode  *int       `json:"exitCode"`
	UsageJson string     `json:"usageJson"`
	CreatedAt *time.Time `json:"createdAt"`
}
