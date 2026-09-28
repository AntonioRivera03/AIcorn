package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

// AgentTaskPatch is deliberately narrower than a human's task edit. Properties
// are patched independently and stage transitions retain their explicit CAS.
type AgentTaskPatch struct {
	Name        *string
	Priority    *string
	Assignee    *string
	Body        *string // Already converted to Plate before entering the write transaction.
	ChecklistID *int
	TypeID      *int
}

// NotFoundError is what an agent-supplied id (task, checklist, stage, or task
// type) resolves to when it doesn't exist — or, for a task, when it exists
// outside this run's project scope, which must read identically so a scoped
// run can never tell "no such task" apart from "that task belongs to a
// project you can't see". The functions below construct it once, at the
// query site that knows which entity was being looked up, instead of letting
// a raw sql.ErrNoRows (or a driver constraint error) leak up to the MCP tool
// caller as "sql: no rows in result set".
type NotFoundError struct {
	Entity string
	ID     int
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s %d not found", e.Entity, e.ID)
}

func NotFound(entity string, id int) error {
	return NotFoundError{Entity: entity, ID: id}
}

func agentTaskProject(tx *sql.Tx, task, scope, job int, chatTurn ...int) (int, error) {
	var project int
	err := tx.QueryRow("SELECT c.project FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND (?=0 OR c.project=?)", task, scope, scope).Scan(&project)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, NotFound("task", task)
		}
		return 0, err
	}
	return project, taskownership.Check(tx, task, job, chatTurn...)
}

func (r *TaskRepo) UpdateByAgent(task, scope, job int, patch AgentTaskPatch, chatTurn ...int) (bool, error) {
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	project, err := agentTaskProject(tx, task, scope, job, chatTurn...)
	if err != nil {
		return false, err
	}
	if patch.Name != nil && len(*patch.Name) > 500 || patch.Assignee != nil && len(*patch.Assignee) > 300 || patch.Body != nil && len(*patch.Body) > 256000 {
		return false, fmt.Errorf("task text is too long")
	}
	if patch.Priority != nil {
		switch *patch.Priority {
		case "Urgent", "High", "Medium", "Low":
		default:
			return false, fmt.Errorf("unknown task priority")
		}
	}
	if patch.ChecklistID != nil {
		var dest int
		if err = tx.QueryRow("SELECT project FROM checklist WHERE id=? AND (?=0 OR project=?)", *patch.ChecklistID, scope, scope).Scan(&dest); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, NotFound("checklist", *patch.ChecklistID)
			}
			return false, err
		}
		var valid bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task t JOIN stage s ON s.id=t.stage JOIN project p ON p.workflow=s.workflow WHERE t.id=? AND p.id=?)", task, dest).Scan(&valid); err != nil {
			return false, err
		}
		if !valid {
			return false, fmt.Errorf("the task's current stage is outside the destination project's workflow")
		}
		project = dest
	}
	if patch.TypeID != nil {
		var exists bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task_type WHERE id=?)", *patch.TypeID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, NotFound("task type", *patch.TypeID)
		}
		var valid bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM project_task_type WHERE project=? AND task_type=?)", project, *patch.TypeID).Scan(&valid); err != nil {
			return false, err
		}
		if !valid {
			return false, fmt.Errorf("task type is not enabled for this project")
		}
	}
	res, err := tx.Exec(`UPDATE task SET name=COALESCE(?,name),priority=COALESCE(?,priority),assignee=COALESCE(?,assignee),body=COALESCE(?,body),checklist=COALESCE(?,checklist),type=COALESCE(?,type) WHERE id=?`, patch.Name, patch.Priority, patch.Assignee, patch.Body, patch.ChecklistID, patch.TypeID, task)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, tx.Commit()
}

func (r *TaskRepo) MoveByAgent(task, scope, job, from, to int, chatTurn ...int) (bool, error) {
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	project, err := agentTaskProject(tx, task, scope, job, chatTurn...)
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM stage WHERE id=?)", to).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, NotFound("stage", to)
	}
	var valid bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM stage s JOIN project p ON p.workflow=s.workflow WHERE s.id=? AND p.id=?)", to, project).Scan(&valid); err != nil {
		return false, err
	}
	if !valid {
		return false, fmt.Errorf("stage is outside the project's workflow")
	}
	res, err := tx.Exec(`UPDATE task SET stage=?,timeCompleted=CASE WHEN (SELECT type FROM stage WHERE id=?)='done' THEN CURRENT_TIMESTAMP ELSE NULL END WHERE id=? AND stage=?`, to, to, task, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, tx.Commit()
}

// CreateFromChat validates project references and the live Chatter turn while
// holding the same writer lock used by cancellation and task ownership changes.
func (r *TaskRepo) CreateFromChat(project, turn int, task *models.ChecklistTask) (*models.ChecklistTask, error) {
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = taskownership.CheckChat(tx, project, turn); err != nil {
		return nil, err
	}
	if len(task.Name) > 500 || len(task.Assignee) > 300 {
		return nil, fmt.Errorf("task text is too long")
	}
	switch task.Priority {
	case "Urgent", "High", "Medium", "Low":
	default:
		return nil, fmt.Errorf("invalid priority")
	}
	var checklistExists, stageExists bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM checklist WHERE id=?)", task.Checklist).Scan(&checklistExists); err != nil {
		return nil, err
	}
	if !checklistExists {
		return nil, NotFound("checklist", task.Checklist)
	}
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM stage WHERE id=?)", task.Stage).Scan(&stageExists); err != nil {
		return nil, err
	}
	if !stageExists {
		return nil, NotFound("stage", task.Stage)
	}
	var valid bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM checklist c JOIN project p ON p.id=c.project JOIN stage s ON s.workflow=p.workflow WHERE c.id=? AND p.id=? AND s.id=?)", task.Checklist, project, task.Stage).Scan(&valid); err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("checklist or stage is outside this project")
	}
	if task.Type.ID == 0 {
		if err = tx.QueryRow("SELECT tt.id FROM task_type tt JOIN project_task_type pt ON pt.task_type=tt.id WHERE pt.project=? ORDER BY tt.isDefault DESC,tt.id LIMIT 1", project).Scan(&task.Type.ID); err != nil {
			return nil, err
		}
	} else {
		var typeExists bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task_type WHERE id=?)", task.Type.ID).Scan(&typeExists); err != nil {
			return nil, err
		}
		if !typeExists {
			return nil, NotFound("task type", task.Type.ID)
		}
	}
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM project_task_type WHERE project=? AND task_type=?)", project, task.Type.ID).Scan(&valid); err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("task type is not enabled for this project")
	}
	if task.TimePlannedStart != nil && task.TimePlannedEnd != nil && task.TimePlannedEnd.Before(*task.TimePlannedStart) {
		return nil, fmt.Errorf("planned end is before start")
	}
	err = tx.QueryRow(`INSERT INTO task(checklist,stage,type,name,priority,assignee,timePlannedStart,timePlannedEnd,hasTimePlannedStart,hasTimePlannedEnd,timeCompleted) VALUES(?,?,?,?,?,?,?,?,?,?,CASE WHEN (SELECT type FROM stage WHERE id=?)='done' THEN CURRENT_TIMESTAMP ELSE NULL END) RETURNING id`, task.Checklist, task.Stage, task.Type.ID, task.Name, task.Priority, task.Assignee, task.TimePlannedStart, task.TimePlannedEnd, task.HasTimePlannedStart, task.HasTimePlannedEnd, task.Stage).Scan(&task.ID)
	if err != nil {
		return nil, err
	}
	return task, tx.Commit()
}

// ValidateCreateRefs confirms a checklist, stage, and (if given) task type
// exist before an unscoped agent's create_task call. CreateTask — the plain
// insert this path uses, shared with the REST API — otherwise leans on the
// checklist/stage/type foreign keys alone, which fail with an opaque driver
// error ("constraint failed: FOREIGN KEY constraint failed") instead of
// naming the id an agent got wrong.
func (r *TaskRepo) ValidateCreateRefs(checklistID, stageID, typeID int) error {
	var exists bool
	if err := r.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM checklist WHERE id=?)", checklistID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return NotFound("checklist", checklistID)
	}
	if err := r.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM stage WHERE id=?)", stageID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return NotFound("stage", stageID)
	}
	if typeID == 0 {
		return nil
	}
	if err := r.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM task_type WHERE id=?)", typeID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return NotFound("task type", typeID)
	}
	return nil
}
