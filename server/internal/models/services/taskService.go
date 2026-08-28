package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
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

func (s *TaskService) personaNameForID(personaID int) (string, error) {
	repo := s.getPersonaRepo()
	if repo == nil {
		return "", errors.New("persona repo not configured")
	}
	p, err := repo.FindOne(personaID)
	if err != nil {
		return "", err
	}
	return p.Name, nil
}

func nullableIntArg(v *int) any {
	if v != nil {
		return *v
	}
	return nil
}

func (s *TaskService) enqueueForAssignee(taskID int, assignee string, fromStage *int, toStage *int) error {
	pid, err := s.personaIDForAssignee(assignee)
	if err != nil {
		return err
	}
	if pid == nil {
		return nil
	}
	if s.AgentJobService == nil || s.AgentJobService.JobRepo == nil {
		return nil
	}
	existing, err := s.AgentJobService.JobRepo.FindPendingByTask(taskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existing != nil {
		return nil
	}
	_, err = s.AgentJobService.Enqueue(taskID, *pid, fromStage, toStage)
	if err != nil {
		if errors.Is(err, ErrJobAlreadyPending) || errors.Is(err, ErrInvalidPersona) {
			return nil
		}
		// If duplicate pending due to race, ignore
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
	}
	return err
}

func (s *TaskService) TransitionStage(taskId, fromStage, toStage int) (bool, error) {
	// Stage-persona is now only for auto-assigning assignee; job enqueue is via assignee.
	var personaID *int
	var personaName string
	if s.StagePersonaRepo != nil {
		sp, err := s.StagePersonaRepo.FindByStage(toStage)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return false, err
			}
		} else {
			pid := sp.PersonaID
			personaID = &pid
			name, err := s.personaNameForID(pid)
			if err != nil {
				// Preserve atomic rollback semantics for invalid binding (FK 999 test).
				// If persona not found, we cannot set assignee; return error without moving stage.
				if errors.Is(err, sql.ErrNoRows) {
					return false, fmt.Errorf("persona %d not found: %w", pid, err)
				}
				return false, err
			}
			personaName = name
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
	res, err := tx.Exec(`UPDATE task SET stage = ?, assignee = ? WHERE id = ? AND stage = ?;`, toStage, personaName, taskId, fromStage)
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
	// Idempotent: only insert if no pending already exists for this task (within Tx).
	var pendingCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM agent_job WHERE task = ? AND status = 'pending';`, taskId).Scan(&pendingCount); err != nil {
		return false, err
	}
	if pendingCount == 0 {
		if _, err := tx.Exec(`INSERT INTO agent_job (task, persona, status, fromStage, toStage) VALUES (?, ?, 'pending', ?, ?);`, taskId, *personaID, fromStage, toStage); err != nil {
			return false, err
		}
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
	if newTask != nil && strings.TrimSpace(newTask.Assignee) != "" {
		if err := s.enqueueForAssignee(newTask.ID, newTask.Assignee, nil, &newTask.Stage); err != nil {
			log.Printf("enqueueForAssignee after CreateChecklistTask %d assignee %q: %v", newTask.ID, newTask.Assignee, err)
		}
	}
	return newTask, nil
}

func (s *TaskService) UpdateTask(updatedTask *models.ChecklistTask) (bool, error) {
	var oldAssignee string
	var oldStage int
	var oldFound bool
	if updatedTask != nil && s.TaskRepo != nil && s.TaskRepo.DB != nil {
		var assignee sql.NullString
		var stage sql.NullInt64
		err := s.TaskRepo.DB.QueryRow(`SELECT COALESCE(assignee,''), stage FROM task WHERE id = ?;`, updatedTask.ID).Scan(&assignee, &stage)
		if err == nil {
			if assignee.Valid {
				oldAssignee = assignee.String
			}
			if stage.Valid {
				oldStage = int(stage.Int64)
			}
			oldFound = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	success, err := s.TaskRepo.UpdateTask(updatedTask)
	if err != nil {
		return false, err
	}
	if success && updatedTask != nil && strings.TrimSpace(updatedTask.Assignee) != "" {
		if !oldFound || updatedTask.Assignee != oldAssignee {
			pid, err := s.personaIDForAssignee(updatedTask.Assignee)
			if err != nil {
				log.Printf("personaIDForAssignee after UpdateTask %d: %v", updatedTask.ID, err)
			} else if pid != nil {
				fromStage := oldStage
				toStage := updatedTask.Stage
				var fromPtr *int
				var toPtr *int
				if oldFound {
					fromPtr = &fromStage
				}
				toPtr = &toStage
				if err := s.enqueueForAssignee(updatedTask.ID, updatedTask.Assignee, fromPtr, toPtr); err != nil {
					log.Printf("enqueueForAssignee after UpdateTask %d: %v", updatedTask.ID, err)
				}
			}
		}
	}
	return success, nil
}

func (s *TaskService) UpdateTaskProperties(updatedTask *models.ChecklistTask) (bool, error) {
	var oldAssignee string
	var oldFound bool
	if updatedTask != nil && s.TaskRepo != nil && s.TaskRepo.DB != nil {
		var assignee sql.NullString
		err := s.TaskRepo.DB.QueryRow(`SELECT COALESCE(assignee,'') FROM task WHERE id = ?;`, updatedTask.ID).Scan(&assignee)
		if err == nil {
			if assignee.Valid {
				oldAssignee = assignee.String
			}
			oldFound = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	success, err := s.TaskRepo.UpdateTaskProperties(updatedTask)
	if err != nil {
		return false, err
	}
	if success && updatedTask != nil && strings.TrimSpace(updatedTask.Assignee) != "" {
		if !oldFound || updatedTask.Assignee != oldAssignee {
			pid, err := s.personaIDForAssignee(updatedTask.Assignee)
			if err != nil {
				log.Printf("personaIDForAssignee after UpdateTaskProperties %d: %v", updatedTask.ID, err)
			} else if pid != nil {
				// Stage unchanged in this path (UpdateTaskProperties excludes stage), so use current task Stage.
				toStage := updatedTask.Stage
				if err := s.enqueueForAssignee(updatedTask.ID, updatedTask.Assignee, nil, &toStage); err != nil {
					log.Printf("enqueueForAssignee after UpdateTaskProperties %d: %v", updatedTask.ID, err)
				}
			}
		}
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

	rawStage, hasStage := filtered["stage"]
	var destStage int
	var stageOk bool
	if hasStage {
		destStage, stageOk = parseStageID(rawStage)
	}
	// Resolve stage persona for auto-assign
	var stagePersonaID *int
	var stagePersonaName string
	if stageOk && s.StagePersonaRepo != nil {
		sp, err := s.StagePersonaRepo.FindByStage(destStage)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return models.BulkResult{}, err
			}
		} else {
			pid := sp.PersonaID
			stagePersonaID = &pid
			name, err := s.personaNameForID(pid)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return models.BulkResult{}, fmt.Errorf("persona %d not found: %w", pid, err)
				}
				return models.BulkResult{}, err
			}
			stagePersonaName = name
		}
	}
	// Resolve assignee persona if no stage persona takes precedence
	var assigneePersonaID *int
	var assigneeStr string
	var hasAssignee bool
	if rawAssignee, ok := filtered["assignee"]; ok {
		hasAssignee = true
		switch v := rawAssignee.(type) {
		case string:
			assigneeStr = v
		default:
			assigneeStr = fmt.Sprint(v)
		}
		if stagePersonaID == nil {
			pid, err := s.personaIDForAssignee(assigneeStr)
			if err != nil {
				return models.BulkResult{}, err
			}
			assigneePersonaID = pid
		}
	}

	var enqueuePersonaID *int
	var enqueueToStage *int
	if stagePersonaID != nil {
		filtered["assignee"] = stagePersonaName
		filtered["stage"] = destStage
		enqueuePersonaID = stagePersonaID
		tmp := destStage
		enqueueToStage = &tmp
	} else if hasAssignee && assigneePersonaID != nil {
		enqueuePersonaID = assigneePersonaID
		if stageOk {
			tmp := destStage
			enqueueToStage = &tmp
		}
	} else {
		// No persona-triggered enqueue; simple update.
		affected, err := s.TaskRepo.UpdateManyFields(ids, filtered)
		if err != nil {
			return models.BulkResult{}, err
		}
		return models.BulkResult{
			Success: affected,
			Skipped: len(ids) - affected,
		}, nil
	}

	// Build SET clause for Tx including auto-assigned assignee.
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
	if affected > 0 && enqueuePersonaID != nil {
		// Bulk enqueue with idempotency: only for tasks not already pending.
		// Use INSERT ... SELECT with NOT IN filter.
		ph, phArgs := intIdPlaceholders(ids)
		// Need two copies of ids for IN and NOT IN
		insertQuery := `INSERT INTO agent_job (task, persona, status, fromStage, toStage)
			SELECT id, ?, 'pending', NULL, ? FROM task WHERE id IN (` + ph + `) AND id NOT IN (SELECT task FROM agent_job WHERE status = 'pending' AND task IN (` + ph + `))`
		allArgs := []any{*enqueuePersonaID, nullableIntArg(enqueueToStage)}
		allArgs = append(allArgs, phArgs...)
		allArgs = append(allArgs, phArgs...)
		if _, err := tx.Exec(insertQuery, allArgs...); err != nil {
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

func intIdPlaceholders(ids []int) (string, []any) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders, args
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
