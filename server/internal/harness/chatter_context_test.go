package harness

import (
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

// A #123 mention gives Chatter more than the bare id: BuildContext folds the
// validated referenced tasks straight into the turn's user payload.
func TestProjectChatContextIncludesReferencedTasks(t *testing.T) {
	req := testRequest("codex")
	req.ProjectID = 7
	req.Instruction = "What's the status of #12?"
	req.ProjectChat = &models.ProjectChatTurn{
		TaskIDs: []int{12},
		ReferencedTasks: []models.ReferencedTask{
			{ID: 12, Title: "Fix login", Stage: "Doing", Type: "Dev", Owner: "Conductor", State: "working", Excerpt: "Users can't sign in on Safari."},
		},
	}

	developer, user := BuildContext(RunSpec{Request: req})
	if !strings.Contains(developer, "Chatter") {
		t.Fatalf("developer instructions should be Chatter's: %q", developer)
	}
	for _, want := range []string{`"id":12`, `"title":"Fix login"`, `"stage":"Doing"`, `"owner":"Conductor"`, `"state":"working"`, `"excerpt":"Users can't sign in on Safari."`} {
		if !strings.Contains(user, want) {
			t.Fatalf("payload missing %s: %s", want, user)
		}
	}
	if strings.Contains(user, "referencedTaskIds") {
		t.Fatalf("bare ids should be superseded by the richer referencedTasks block: %s", user)
	}
}
