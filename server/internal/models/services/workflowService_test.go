package services

import (
	"database/sql"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	_ "modernc.org/sqlite"
)

func setupWorkflowServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE workflow (
		    id INTEGER PRIMARY KEY,
		    name VARCHAR NOT NULL,
		    description VARCHAR,
		    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		    timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE stage (
		    id INTEGER PRIMARY KEY,
		    workflow INTEGER NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
		    name VARCHAR NOT NULL,
		    description VARCHAR,
		    color VARCHAR NOT NULL,
		    icon VARCHAR NOT NULL,
		    position INTEGER NOT NULL,
		    type TEXT NOT NULL CHECK(type IN ('open', 'todo', 'doing', 'done')),
		    timeCreated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		    timeModified TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE task (id INTEGER PRIMARY KEY, stage INTEGER);
		CREATE TABLE persona (id INTEGER PRIMARY KEY, name VARCHAR, harness VARCHAR, model VARCHAR, agent VARCHAR);
		CREATE TABLE stage_persona (stage_id INTEGER PRIMARY KEY, persona_id INTEGER);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// wantStages asserts a workflow's stages, in position order, are Open,
// Doing, Review, Done (issue #32: Conductor needs a stage to hand finished
// work to, and the classic Open, Doing, Done starter had none).
func wantStages(t *testing.T, stages []models.Stage) {
	t.Helper()
	want := []struct{ name, typ string }{
		{"Open", "open"},
		{"Doing", "doing"},
		{"Review", "todo"},
		{"Done", "done"},
	}
	if len(stages) != len(want) {
		t.Fatalf("stages=%+v", stages)
	}
	for i, w := range want {
		if stages[i].Name != w.name || stages[i].Type != w.typ || stages[i].Position != i+1 {
			t.Fatalf("stage %d: got name=%q type=%q position=%d, want name=%q type=%q position=%d",
				i, stages[i].Name, stages[i].Type, stages[i].Position, w.name, w.typ, i+1)
		}
	}
}

func TestDefaultWorkflowStagesIncludeReview(t *testing.T) {
	wantStages(t, defaultWorkflowStages())
}

func TestCreateWorkflowUsesOpenDoingReviewDone(t *testing.T) {
	db := setupWorkflowServiceTestDB(t)
	workflowRepo := &repos.WorkflowRepo{DB: db}
	stageRepo := &repos.StageRepo{DB: db}
	s := &WorkflowService{WorkflowRepo: workflowRepo, StageRepo: stageRepo}

	id, err := s.CreateWorkflow()
	if err != nil {
		t.Fatal(err)
	}
	stages, err := stageRepo.ByWorkflow(int(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	wantStages(t, stages)
}

func TestEnsureStarterWorkflowUsesOpenDoingReviewDoneAndIsIdempotent(t *testing.T) {
	db := setupWorkflowServiceTestDB(t)
	workflowRepo := &repos.WorkflowRepo{DB: db}
	stageRepo := &repos.StageRepo{DB: db}
	s := &WorkflowService{WorkflowRepo: workflowRepo, StageRepo: stageRepo}

	if err := s.EnsureStarterWorkflow(); err != nil {
		t.Fatal(err)
	}
	workflows, err := workflowRepo.All()
	if err != nil || len(workflows) != 1 {
		t.Fatalf("workflows=%+v err=%v", workflows, err)
	}
	stages, err := stageRepo.ByWorkflow(workflows[0].ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantStages(t, stages)

	// A workspace that already has a workflow keeps it untouched.
	if err := s.EnsureStarterWorkflow(); err != nil {
		t.Fatal(err)
	}
	workflows, err = workflowRepo.All()
	if err != nil || len(workflows) != 1 {
		t.Fatalf("starter workflow duplicated: %+v %v", workflows, err)
	}
}
