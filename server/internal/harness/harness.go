package harness

import (
	"context"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

// RunSpec carries the immutable request and its isolated working directory.
type RunSpec struct {
	Request    *models.AIRunRequest
	OnProgress func(string, string) error
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
}

type Harness interface {
	Run(context.Context, RunSpec) (RunResult, error)
}
