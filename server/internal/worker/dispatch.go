package worker

import (
	"context"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"os"
	"time"
)

func (w *Worker) runDispatch(ctx context.Context) (bool, error) {
	req, candidates, err := w.Conductor.PrepareDispatch(ctx)
	if err != nil || req == nil {
		return false, err
	}
	result := harness.RunResult{}
	dir, err := os.MkdirTemp("", "aycorn-dispatch-")
	if err == nil {
		defer os.RemoveAll(dir)
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		watcherDone := make(chan struct{})
		go func() {
			defer close(watcherDone)
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-runCtx.Done():
					return
				case <-ticker.C:
					settings, _, e := w.Conductor.Repo.Settings(req.ProjectID)
					if e != nil || !settings.Enabled {
						cancel()
						return
					}
				}
			}
		}()
		result, err = w.Harness.Run(runCtx, harness.RunSpec{Request: req, WorkDir: dir})
		close(done)
		cancel()
		<-watcherDone
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	if saveErr := w.Conductor.Repo.FinishDispatch(req.DispatchID, result.Output, result.SessionID, message); saveErr != nil {
		return true, saveErr
	}
	// Unselected snapshot candidates wait for a deliberate recheck, never a hot
	// loop of paid model calls. New arrivals during selection are unaffected.
	for _, t := range candidates {
		state, note := "held", "Conductor did not start this task. Recheck when ready."
		if err != nil {
			state, note = "failed", "Task selection failed: "+message
		}
		if saveErr := w.Conductor.Repo.SetState(t, state, note); saveErr != nil {
			return true, saveErr
		}
	}
	return true, err
}
