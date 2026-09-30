package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

func callConductor(t *testing.T, a *app, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
	if r.Code != want {
		t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
	}
	return r
}

func decodeBoard(t *testing.T, r *httptest.ResponseRecorder) services.ConductorBoard {
	t.Helper()
	var board services.ConductorBoard
	if err := json.Unmarshal(r.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	return board
}

// The base fixture's workflow (see conductorTestApp) is Open, Plan, Doing,
// Review, Done — a resolvable pair exists without anyone visiting settings.
func TestConductorBoardAutoPicksStagesWithoutWriting(t *testing.T) {
	a := conductorTestApp(t)
	db := a.aiService.Jobs.DB

	board := decodeBoard(t, callConductor(t, a, "GET", "/api/project/1/settings/conductor", "", 200))
	if !board.StagesAutoPicked || board.Settings.WorkingStage != 3 || board.Settings.CompletionStage != 4 || board.ConfigurationError != "" {
		t.Fatalf("bad auto pick: %+v", board)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM conductor_project WHERE project=1`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("GET persisted a row")
	}

	// Sending a task is an actual use: it persists the automatic pick.
	callConductor(t, a, "POST", "/api/project/1/conductor/bulk", `{"ids":[1],"action":"send"}`, 200)
	if err := db.QueryRow(`SELECT COUNT(*) FROM conductor_project WHERE project=1`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatal("send did not persist the pick")
	}

	board = decodeBoard(t, callConductor(t, a, "GET", "/api/project/1/settings/conductor", "", 200))
	if board.StagesAutoPicked || board.Settings.WorkingStage != 3 || board.Settings.CompletionStage != 4 {
		t.Fatalf("persisted pick not reflected: %+v", board)
	}
}

// A classic Open, Doing, Done workflow has a working stage but nothing
// Conductor can hand finished work to. The board and Settings should say so
// and offer AddReviewStage, which fixes it with one call.
func TestConductorNoFinishStageOffersAddReviewStage(t *testing.T) {
	a := conductorTestApp(t)
	db := a.aiService.Jobs.DB
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(3,'Classic')`,
		`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(7,3,'Open','open','gray','circle',1),(8,3,'Doing','doing','gray','circle',2),(9,3,'Done','done','gray','circle',3)`,
		`INSERT INTO project(id,name,pinned,workflow,defaultView) VALUES(2,'Classic project',0,3,'')`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(2,2,'Tasks',1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	board := decodeBoard(t, callConductor(t, a, "GET", "/api/project/2/settings/conductor", "", 200))
	if board.ConfigurationError == "" || !board.CanAddReviewStage || board.Settings.WorkingStage != 0 || board.Settings.CompletionStage != 0 {
		t.Fatalf("expected a no-candidate configuration error: %+v", board)
	}
	callConductor(t, a, "PUT", "/api/project/2/settings/conductor", `{"enabled":true}`, 400)

	r := callConductor(t, a, "POST", "/api/project/2/settings/conductor/review-stage", "", 201)
	var added models.Stage
	if err := json.Unmarshal(r.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Name != "Review" || added.Type != "todo" || added.Position != 3 || added.Workflow != 3 {
		t.Fatalf("bad review stage: %+v", added)
	}

	board = decodeBoard(t, callConductor(t, a, "GET", "/api/project/2/settings/conductor", "", 200))
	if board.ConfigurationError != "" || !board.StagesAutoPicked || board.Settings.WorkingStage != 8 || board.Settings.CompletionStage != added.ID {
		t.Fatalf("expected the new Review stage to resolve: %+v %+v", board, added)
	}
}

// A workflow with no Doing stage at all can't anchor AddReviewStage.
func TestConductorNoWorkingStageRejectsAddReviewStage(t *testing.T) {
	a := conductorTestApp(t)
	// Workflow 2 (see conductorTestApp) has a single Open stage and no
	// project of its own; give it one so AddReviewStage has a project to act on.
	db := a.aiService.Jobs.DB
	for _, q := range []string{
		`INSERT INTO project(id,name,pinned,workflow,defaultView) VALUES(2,'No working stage',0,2,'')`,
		`INSERT INTO checklist(id,project,name,isDefault) VALUES(2,2,'Tasks',1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	board := decodeBoard(t, callConductor(t, a, "GET", "/api/project/2/settings/conductor", "", 200))
	if board.ConfigurationError == "" || board.CanAddReviewStage {
		t.Fatalf("expected a no-working-stage configuration error without an Add Review Stage offer: %+v", board)
	}
	callConductor(t, a, "POST", "/api/project/2/settings/conductor/review-stage", "", 400)
}
