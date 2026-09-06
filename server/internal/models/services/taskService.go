package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

var ErrStageConflict = errors.New("task is not currently in the expected stage")

var (
	ErrNoPersonaBound    = errors.New("task is not assigned to an agent — set assignee to a persona name")
	ErrRepoPathMissing   = errors.New("project has no repo folder linked")
	ErrRepoInvalid       = errors.New("linked repo folder is not a valid git repository")
	ErrJobAlreadyPending = errors.New("agent job already pending for this task")
)

type TaskService struct {
	TaskRepo         *repos.TaskRepo
	TaskTypeRepo     *repos.TaskTypeRepo
	AgentJobService  *AgentJobService
	StagePersonaRepo *repos.StagePersonaRepo
	ProjectRepo      *repos.ProjectRepo
	PersonaRepo      *repos.PersonaRepo
}

func (s *TaskService) getPersonaRepo() *repos.PersonaRepo {
	if s.PersonaRepo != nil {
		return s.PersonaRepo
	}
	if s.AgentJobService != nil {
		return s.AgentJobService.PersonaRepo
	}
	return nil
}

func (s *TaskService) personaIDForAssignee(assignee string) (*int, error) {
	trimmed := strings.TrimSpace(assignee)
	if trimmed == "" {
		return nil, nil
	}
	repo := s.getPersonaRepo()
	if repo == nil {
		return nil, nil
	}
	p, err := repo.FindByName(trimmed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p.ID, nil
}

func (s *TaskService) TransitionStage(taskId, fromStage, toStage int) (bool, error) {
	ok, err := s.TaskRepo.CompareAndSwapStage(taskId, fromStage, toStage)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrStageConflict
	}
	return true, nil
}

func (s *TaskService) GetAllTasks(filters *repos.TaskFilters) ([]models.TaskWithProject, error) {
	return s.TaskRepo.AllTasks(filters)
}

func (s *TaskService) GetTaskFacets() (*models.TaskFacets, error) {
	return s.TaskRepo.TaskFacets()
}

func (s *TaskService) GetTask(taskId int) (*models.TaskWithProject, error) {
	task, err := s.TaskRepo.FindOneWithProject(taskId)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskService) GetTaskBody(taskId int) (string, error) {
	taskBody, err := s.TaskRepo.GetTaskBody(taskId)
	if err != nil {
		return models.EmptyBody, err
	}

	return taskBody, nil
}

func (s *TaskService) CreateChecklistTask(task *models.ChecklistTask) (*models.ChecklistTask, error) {
	if task.Type.ID == 0 && s.TaskTypeRepo != nil {
		defaultID, err := s.TaskTypeRepo.DefaultTypeID()
		if err != nil {
			return nil, err
		}
		task.Type.ID = defaultID
	}
	task.Body = models.NormalizeBody(task.Body)

	return s.TaskRepo.CreateTask(task)
}

func (s *TaskService) UpdateTask(task *models.ChecklistTask) (bool, error) {
	return s.TaskRepo.UpdateTask(task)
}

func (s *TaskService) UpdateTaskProperties(task *models.ChecklistTask) (bool, error) {
	return s.TaskRepo.UpdateTaskProperties(task)
}

func (s *TaskService) UpdateTaskBody(taskId int, body string) (bool, error) {
	success, err := s.TaskRepo.UpdateTaskBody(taskId, body)
	if err != nil {
		return false, err
	}

	return success, nil
}

func (s *TaskService) DeleteTask(taskId int) (bool, error) {
	success, err := s.TaskRepo.DeleteTask(taskId)
	if err != nil {
		return false, err
	}

	return success, nil
}

var bulkTaskUpdatableColumns = map[string]string{
	"Stage":               "stage",
	"Priority":            "priority",
	"Type":                "type",
	"Assignee":            "assignee",
	"Checklist":           "checklist",
	"TimePlannedStart":    "timePlannedStart",
	"TimePlannedEnd":      "timePlannedEnd",
	"HasTimePlannedStart": "hasTimePlannedStart",
	"HasTimePlannedEnd":   "hasTimePlannedEnd",
}

func (s *TaskService) BulkUpdate(ids []int, changes map[string]any) (models.BulkResult, error) {
	ids = dedupeInts(ids)
	if len(ids) == 0 {
		return models.BulkResult{}, nil
	}

	filtered := map[string]any{}
	for jsonKey, dbCol := range bulkTaskUpdatableColumns {
		if v, ok := changes[jsonKey]; ok {
			filtered[dbCol] = v
		}
	}
	if len(filtered) == 0 {
		return models.BulkResult{Skipped: len(ids)}, nil
	}

	affected, err := s.TaskRepo.UpdateManyFields(ids, filtered)
	if err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{Success: affected, Skipped: len(ids) - affected}, nil
}

func (s *TaskService) BulkDelete(ids []int) (models.BulkResult, error) {
	ids = dedupeInts(ids)
	if len(ids) == 0 {
		return models.BulkResult{}, nil
	}
	affected, err := s.TaskRepo.DeleteMany(ids)
	if err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{
		Success: affected,
		Skipped: len(ids) - affected,
	}, nil
}

// RequestAgent manually enqueues a job for a task whose assignee is a persona.
// It resolves persona via task.assignee exact match, validates RepoPath, checks idempotency,
// then inserts pending agent_job. Does NOT change task.stage or assignee.
func (s *TaskService) RequestAgent(taskID int) (*models.AgentJob, error) {
	task, err := s.TaskRepo.FindOneWithProject(taskID)
	if err != nil {
		return nil, err
	}
	pid, err := s.personaIDForAssignee(task.Assignee)
	if err != nil {
		return nil, err
	}
	if pid == nil {
		return nil, ErrNoPersonaBound
	}
	personaID := *pid

	// Project RepoPath guard — fail fast before enqueue
	if s.ProjectRepo != nil {
		proj, err := s.ProjectRepo.FindOne(task.ProjectID)
		if err != nil {
			return nil, err
		}
		repoPath := strings.TrimSpace(proj.RepoPath)
		if repoPath == "" {
			return nil, ErrRepoPathMissing
		}
		info, statErr := os.Stat(repoPath)
		if statErr != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: %s", ErrRepoInvalid, repoPath)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--show-toplevel")
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrRepoInvalid, repoPath)
		}
	}

	// Idempotent: if a pending job already exists for this task, reject
	if s.AgentJobService != nil && s.AgentJobService.JobRepo != nil {
		existing, err := s.AgentJobService.JobRepo.FindPendingByTask(taskID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if existing != nil {
			return nil, ErrJobAlreadyPending
		}
	}

	if s.AgentJobService == nil {
		return nil, errors.New("agent job service not configured")
	}
	toStage := task.Stage
	return s.AgentJobService.Enqueue(taskID, personaID, nil, &toStage)
}
