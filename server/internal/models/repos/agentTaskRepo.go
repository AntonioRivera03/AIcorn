package repos

import (
	"database/sql"
	"fmt"
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

func agentTaskProject(tx *sql.Tx, task, scope, job int) (int, error) {
	var project int
	err := tx.QueryRow("SELECT c.project FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND (?=0 OR c.project=?)", task, scope, scope).Scan(&project)
	if err != nil {
		return 0, err
	}
	return project, taskownership.Check(tx, task, job)
}

func (r *TaskRepo) UpdateByAgent(task, scope, job int, patch AgentTaskPatch) (bool, error) {
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	project, err := agentTaskProject(tx, task, scope, job)
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

func (r *TaskRepo) MoveByAgent(task, scope, job, from, to int) (bool, error) {
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	project, err := agentTaskProject(tx, task, scope, job)
	if err != nil {
		return false, err
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
