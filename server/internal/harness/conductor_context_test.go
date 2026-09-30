package harness

import (
	"strings"
	"testing"
)

func TestConductorDispatchContextIncludesOnlyProjectGuidance(t *testing.T) {
	req := testRequest("codex")
	req.DispatchID, req.ProjectID = 1, 7

	_, user := BuildContext(RunSpec{Request: req})
	if user != "Project ID: 7" {
		t.Fatalf("empty guidance should leave only the project: %q", user)
	}

	req.Instruction = "  Start bugs first.  "
	_, user = BuildContext(RunSpec{Request: req})
	if !strings.HasSuffix(user, "Project guidance for choosing tasks:\nStart bugs first.") {
		t.Fatalf("guidance missing: %q", user)
	}
}
