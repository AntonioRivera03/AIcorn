package services

import (
	"context"
	"errors"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

// A workflow with no Doing stage has nowhere for Conductor to move a task
// while an agent works — nothing to auto-pick, and nothing "Add a Review
// stage" can anchor to.
var ErrConductorNoWorkingStage = errors.New("This workflow has no Doing stage, so Conductor has nowhere to put a task while an agent works. Add one, or choose stages in Conductor settings.")

// A workflow can have a working stage but nothing after it Conductor can hand
// finished work to — the classic Open, Doing, Done. AddReviewStage fixes this
// with one click.
var ErrConductorNoFinishStage = errors.New("This workflow has no stage for finished work — Conductor can't hand a task back once an agent is done. Add a Review stage, or choose one in Conductor settings.")

// isConductorConfigError reports whether err is one Conductor's own callers
// should treat as "not configured yet" (a message to show, not a fault) —
// either the resolver found no candidate, or a saved choice no longer holds.
func isConductorConfigError(err error) bool {
	return errors.Is(err, repos.ErrConductorConfig) || errors.Is(err, ErrConductorNoWorkingStage) || errors.Is(err, ErrConductorNoFinishStage)
}

func looksLikeReviewStage(name string) bool {
	return strings.Contains(strings.ToLower(name), "review")
}

// resolveConductorStages picks Conductor's working and finish stage from a
// workflow's stages, in position order, for a project that hasn't chosen
// either. Working is the first Doing stage. Finish is the first later stage
// that is neither Open nor Done, preferring one whose name looks like a
// review stage — so the classic Open, Doing, Done workflow has no finish
// candidate at all, and callers must offer to add one (see AddReviewStage).
func resolveConductorStages(stages []models.Stage) (working, finish int, err error) {
	for _, stage := range stages {
		if stage.Type == "doing" {
			working = stage.ID
			break
		}
	}
	if working == 0 {
		return 0, 0, ErrConductorNoWorkingStage
	}
	workingPosition := 0
	for _, stage := range stages {
		if stage.ID == working {
			workingPosition = stage.Position
			break
		}
	}
	firstCandidate, reviewCandidate := 0, 0
	for _, stage := range stages {
		if stage.Position <= workingPosition || stage.Type == "open" || stage.Type == "done" {
			continue
		}
		if firstCandidate == 0 {
			firstCandidate = stage.ID
		}
		if reviewCandidate == 0 && looksLikeReviewStage(stage.Name) {
			reviewCandidate = stage.ID
		}
	}
	if reviewCandidate != 0 {
		return working, reviewCandidate, nil
	}
	if firstCandidate != 0 {
		return working, firstCandidate, nil
	}
	return working, 0, ErrConductorNoFinishStage
}

// fillAutoStages fills in an automatic working/finish stage pick when a
// project hasn't chosen either, mutating settings in place. It reports
// whether it changed anything, so callers only persist when it did.
func (s *ConductorService) fillAutoStages(project int, settings *models.ConductorSettings) (bool, error) {
	if settings.WorkingStage > 0 || settings.CompletionStage > 0 {
		return false, nil
	}
	p, err := s.AI.Projects.FindOne(project)
	if err != nil {
		return false, err
	}
	stages, err := s.Stages.ByWorkflow(p.Workflow, 0)
	if err != nil {
		return false, err
	}
	working, finish, err := resolveConductorStages(stages)
	if err != nil {
		return false, err
	}
	settings.WorkingStage, settings.CompletionStage = working, finish
	return true, nil
}

// EffectiveSettings returns a project's Conductor settings with the working
// and finish stage resolved — from what was saved, or an automatic pick —
// without writing anything. autoPicked reports whether the pick was
// automatic. Use this for reads (the board, Settings); EnsureStages persists
// the pick for the call sites that actually need it to run.
func (s *ConductorService) EffectiveSettings(project int) (settings models.ConductorSettings, raw string, autoPicked bool, err error) {
	settings, raw, err = s.Repo.Settings(project)
	if err != nil {
		return settings, raw, false, err
	}
	if settings.WorkingStage > 0 || settings.CompletionStage > 0 {
		return settings, raw, false, s.Repo.ValidateStages(project, settings)
	}
	autoPicked, err = s.fillAutoStages(project, &settings)
	return settings, raw, autoPicked, err
}

// EnsureStages returns a project's effective Conductor settings, persisting
// an automatic stage pick the first time Conductor actually needs it: a
// start, a send, or a Job (enabling or firing one). GET handlers must not
// call this — use EffectiveSettings instead.
func (s *ConductorService) EnsureStages(project int) (models.ConductorSettings, error) {
	settings, raw, autoPicked, err := s.EffectiveSettings(project)
	if err != nil || !autoPicked {
		return settings, err
	}
	if err := s.Repo.SaveSettings(project, settings, raw); err != nil {
		if errors.Is(err, repos.ErrConductorConflict) {
			// Another actual-use call already persisted the same deterministic
			// pick; nothing actually conflicts.
			current, _, settingsErr := s.Repo.Settings(project)
			return current, settingsErr
		}
		return settings, err
	}
	return settings, nil
}

// Configuration reports whether Conductor can run for a project: its
// effective stages, whether they were chosen automatically, whether the
// no-candidate case can be fixed with AddReviewStage, and — if it still
// can't run — why.
func (s *ConductorService) Configuration(project int) (settings models.ConductorSettings, autoPicked, canAddReviewStage bool, configError string, err error) {
	settings, _, autoPicked, resolveErr := s.EffectiveSettings(project)
	if resolveErr != nil {
		if !isConductorConfigError(resolveErr) {
			return settings, autoPicked, false, "", resolveErr
		}
		return settings, autoPicked, errors.Is(resolveErr, ErrConductorNoFinishStage), resolveErr.Error(), nil
	}
	if settings.ConductorAgentID == 0 || settings.TaskAgentID == 0 {
		return settings, autoPicked, false, "Bundled Conductor agents are unavailable.", nil
	}
	for _, id := range []int{settings.ConductorAgentID, settings.TaskAgentID} {
		if _, err := s.AI.ResolveAgent(context.Background(), id); err != nil {
			return settings, autoPicked, false, err.Error(), nil
		}
	}
	return settings, autoPicked, false, "", nil
}

// AddReviewStage inserts a Review stage right after this project's working
// stage — the fix offered for the no-candidate case, a workflow (e.g. the
// classic Open, Doing, Done) with nothing Conductor can hand finished work
// to. It's additive and user-triggered, so it needs no confirmation.
func (s *ConductorService) AddReviewStage(project int) (*models.Stage, error) {
	p, err := s.AI.Projects.FindOne(project)
	if err != nil {
		return nil, err
	}
	stages, err := s.Stages.ByWorkflow(p.Workflow, 0)
	if err != nil {
		return nil, err
	}
	working := 0
	for _, stage := range stages {
		if stage.Type == "doing" {
			working = stage.ID
			break
		}
	}
	if working == 0 {
		return nil, ErrConductorNoWorkingStage
	}
	id, err := s.Stages.InsertAfter(p.Workflow, working, reviewStageDefault.Name, reviewStageDefault.Description, reviewStageDefault.Color, reviewStageDefault.Icon, reviewStageDefault.Type)
	if err != nil {
		return nil, err
	}
	return s.Stages.FindOne(int(id))
}
