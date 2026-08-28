package services

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

var ErrStageConflict = errors.New("task is not currently in the expected stage")

type TaskService struct {
	TaskRepo         *repos.TaskRepo
	TaskTypeRepo     *repos.TaskTypeRepo
	AgentJobService  *AgentJobService
	StagePersonaRepo *repos.StagePersonaRepo
}

func (s *TaskService) TransitionStage(taskId, fromStage, toStage int) (bool, error) {
	// atomic: stage update and job enqueue commit together or roll back together
	var personaID *int
	if s.AgentJobService != nil && s.StagePersonaRepo != nil {
		sp, err := s.StagePersonaRepo.FindByStage(toStage)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return false, err
			}
		} else {
			personaID = &sp.PersonaID
		}
	}
	if personaID == nil {
		ok, err := s.TaskRepo.CompareAndSwapStage(taskId, fromStage, toStage)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, ErrStageConflict
		}
		return true, nil
	}
	tx, err := s.TaskRepo.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE task SET stage = ? WHERE id = ? AND stage = ?;`, toStage, taskId, fromStage)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, ErrStageConflict
	}
	if _, err := tx.Exec(`INSERT INTO agent_job (task, persona, status, fromStage, toStage) VALUES (?, ?, 'pending', ?, ?);`, taskId, *personaID, fromStage, toStage); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func parseStageID(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	case string:
		return 0, false
	default:
		return 0, false
	}
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
	// atomic bulk: UPDATE tasks and INSERT jobs in one TX via INSERT...SELECT, no fan-out
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

	rawStage, hasStage := filtered["stage"]
	var destStage int
	var stageOk bool
	if hasStage {
		destStage, stageOk = parseStageID(rawStage)
	}
	var personaID *int
	if stageOk && s.AgentJobService != nil && s.StagePersonaRepo != nil && s.AgentJobService.JobRepo != nil {
		sp, err := s.StagePersonaRepo.FindByStage(destStage)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return models.BulkResult{}, err
			}
		} else {
			personaID = &sp.PersonaID
		}
	}
	if personaID == nil {
		affected, err := s.TaskRepo.UpdateManyFields(ids, filtered)
		if err != nil {
			return models.BulkResult{}, err
		}
		return models.BulkResult{
			Success: affected,
			Skipped: len(ids) - affected,
		}, nil
	}

	filtered["stage"] = destStage

	setParts := make([]string, 0, len(filtered))
	setArgs := make([]any, 0, len(filtered))
	for col, val := range filtered {
		setParts = append(setParts, col+" = ?")
		setArgs = append(setArgs, val)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	idArgs := make([]any, len(ids))
	for i, id := range ids {
		idArgs[i] = id
	}
	tx, err := s.TaskRepo.DB.Begin()
	if err != nil {
		return models.BulkResult{}, err
	}
	defer tx.Rollback()
	query := "UPDATE task SET " + strings.Join(setParts, ", ") + " WHERE id IN (" + placeholders + ");"
	args := append(setArgs, idArgs...)
	res, err := tx.Exec(query, args...)
	if err != nil {
		return models.BulkResult{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return models.BulkResult{}, err
	}
	if affected > 0 {
		if err := s.AgentJobService.JobRepo.BulkEnqueueForStageTx(tx, ids, *personaID, destStage); err != nil {
			return models.BulkResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{
		Success: int(affected),
		Skipped: len(ids) - int(affected),
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
