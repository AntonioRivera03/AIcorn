package services

import (
	"context"
	"errors"
	"testing"
)

func TestPrepareRejectsUnavailableHarnessBeforeAccessingTask(t *testing.T) {
	for _, provider := range []string{"opencode", "unknown"} {
		_, err := (&AIService{}).Prepare(context.Background(), 1, AIRunInput{Engine: provider})
		if !errors.Is(err, ErrInvalidAIRun) {
			t.Fatalf("provider %s: %v", provider, err)
		}
	}
}
