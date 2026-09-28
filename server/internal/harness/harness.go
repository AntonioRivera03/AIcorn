package harness

import (
	"context"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

// RunSpec carries the immutable request and its isolated working directory.
type RunSpec struct {
	Request    *models.AIRunRequest
	OnProgress func(string, string) error
	OnSession  func(string, string) error
	// OnActivity receives the turn's work so far (thinking and tool calls),
	// at the same pace as OnProgress. Optional.
	OnActivity func([]Activity) error
	JobID      int
	TaskID     int
	WorkDir    string
}

// RunResult retains partial output even when the engine returns an error.
// UsageJson contains aggregated usage reported by the engine, not a spend cap.
type RunResult struct {
	Output    string
	ExitCode  int
	UsageJson string
	SessionID string
	TurnID    string
	// Activity is the turn's thinking and tool calls, in order.
	Activity []Activity
}

type Harness interface {
	Run(context.Context, RunSpec) (RunResult, error)
}
