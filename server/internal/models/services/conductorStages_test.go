package services

import (
	"errors"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

func stage(id int, name, stageType string, position int) models.Stage {
	return models.Stage{ID: id, Name: name, Type: stageType, Position: position}
}

func TestResolveConductorStagesStarterWorkflow(t *testing.T) {
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "Review", "todo", 3),
		stage(4, "Done", "done", 4),
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil || working != 2 || finish != 3 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}

func TestResolveConductorStagesPrefersReviewLikeName(t *testing.T) {
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "QA", "todo", 3),
		stage(4, "In Review", "todo", 4),
		stage(5, "Done", "done", 5),
	}
	working, finish, err := resolveConductorStages(stages)
	// "In Review" is preferred over the earlier-positioned "QA" because it
	// looks like a review stage, even though it isn't first by position.
	if err != nil || working != 2 || finish != 4 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}

func TestResolveConductorStagesWithoutReviewLikeNameUsesFirstCandidate(t *testing.T) {
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "QA", "todo", 3),
		stage(4, "Staging", "todo", 4),
		stage(5, "Done", "done", 5),
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil || working != 2 || finish != 3 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}

func TestResolveConductorStagesNoCandidateAfterWorking(t *testing.T) {
	// The classic Open, Doing, Done workflow: a working stage exists, but
	// nothing after it Conductor can hand finished work to.
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "Done", "done", 3),
	}
	_, _, err := resolveConductorStages(stages)
	if !errors.Is(err, ErrConductorNoFinishStage) {
		t.Fatalf("expected ErrConductorNoFinishStage, got %v", err)
	}
}

func TestResolveConductorStagesNoWorkingStage(t *testing.T) {
	// No Doing stage at all: nothing to anchor a pick on.
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Todo", "todo", 2),
		stage(3, "Done", "done", 3),
	}
	_, _, err := resolveConductorStages(stages)
	if !errors.Is(err, ErrConductorNoWorkingStage) {
		t.Fatalf("expected ErrConductorNoWorkingStage, got %v", err)
	}
}

func TestResolveConductorStagesIgnoresReviewLikeStageBeforeWorking(t *testing.T) {
	// A stage named "Review" that sits before the working stage can't be the
	// finish stage — only stages after Doing are candidates.
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Review", "todo", 2),
		stage(3, "Doing", "doing", 3),
		stage(4, "Staging", "todo", 4),
		stage(5, "Done", "done", 5),
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil || working != 3 || finish != 4 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}

func TestResolveConductorStagesFirstDoingStageIsWorking(t *testing.T) {
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "Also doing", "doing", 3),
		stage(4, "Review", "todo", 4),
		stage(5, "Done", "done", 5),
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil || working != 2 || finish != 4 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}

func TestResolveConductorStagesOnlyExcludesOpenAndDone(t *testing.T) {
	// A later Doing stage isn't Open or Done, so it's still an eligible
	// finish candidate when nothing looks like a review stage.
	stages := []models.Stage{
		stage(1, "Open", "open", 1),
		stage(2, "Doing", "doing", 2),
		stage(3, "Also doing", "doing", 3),
		stage(4, "Done", "done", 4),
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil || working != 2 || finish != 3 {
		t.Fatalf("working=%d finish=%d err=%v", working, finish, err)
	}
}
