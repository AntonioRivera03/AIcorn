package services

import (
	"errors"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

var ErrStageConflict = errors.New("task is not currently in the expected stage")

type TaskService struct {
	TaskRepo     *repos.TaskRepo
	TaskTypeRepo *repos.TaskTypeRepo
}

// TransitionStage moves a task between stages with an optimistic-concurrency
// check. Unlike UpdateTask, this never touches any other column.
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

	newTask, err := s.TaskRepo.CreateTask(task)
	if err != nil {
		return nil, err
	}

	return newTask, nil
}

func (s *TaskService) UpdateTask(updatedTask *models.ChecklistTask) (bool, error) {
	success, err := s.TaskRepo.UpdateTask(updatedTask)
	if err != nil {
		return false, err
	}

	return success, nil
}

// UpdateTaskProperties is UpdateTask without the ability to change stage. See
// TaskRepo.UpdateTaskProperties.
func (s *TaskService) UpdateTaskProperties(updatedTask *models.ChecklistTask) (bool, error) {
	success, err := s.TaskRepo.UpdateTaskProperties(updatedTask)
	if err != nil {
		return false, err
	}

	return success, nil
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
	return models.BulkResult{
		Success: affected,
		Skipped: len(ids) - affected, // non-existent ids: retrying won't help
	}, nil
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
