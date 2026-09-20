package repos

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

func CheckDispatch(q conductorQuerier, project, dispatch int) error {
	var active bool
	err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM conductor_dispatch d JOIN conductor_project p ON p.project=d.project WHERE d.id=? AND d.project=? AND d.status='running' AND p.enabled=1)`, dispatch, project).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return taskownership.ErrNotOwner
	}
	return nil
}

func (r *ConductorRepo) BeginDispatch(project int, req *models.AIRunRequest) (int, error) {
	tx, err := r.beginTransition(project)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var enabled bool
	if err = tx.QueryRow(`SELECT enabled FROM conductor_project WHERE project=?`, project).Scan(&enabled); err != nil {
		return 0, err
	}
	if !enabled {
		return 0, ErrConductorPaused
	}
	var active int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM conductor_dispatch WHERE project=? AND status='running'`, project).Scan(&active); err != nil {
		return 0, err
	}
	if active > 0 {
		return 0, ErrActiveAIRun
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	var id int
	if err = tx.QueryRow(`INSERT INTO conductor_dispatch(project,status,requestJson) VALUES(?,'running',?) RETURNING id`, project, string(raw)).Scan(&id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (r *ConductorRepo) FinishDispatch(id int, output, session, message string) error {
	status := "completed"
	if message != "" {
		status = "failed"
	}
	_, err := r.DB.Exec(`UPDATE conductor_dispatch SET status=?,output=?,sessionId=?,error=? WHERE id=? AND status='running'`, status, output, session, message, id)
	return err
}

func (r *ConductorRepo) InterruptDispatches() error {
	_, err := r.DB.Exec(`UPDATE conductor_dispatch SET status='interrupted',error='Server stopped during task selection' WHERE status='running'`)
	return err
}

func (r *ConductorRepo) DeferTask(project, task, dispatch int, message string) error {
	tx, err := r.beginTransition(project)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = CheckDispatch(tx, project, dispatch); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE conductor_task SET state='needs_context',message=?,updatedAt=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE task=? AND project=? AND state='waiting'`, message, task, project)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConductorConflict
	}
	return tx.Commit()
}

func (r *ConductorRepo) Task(project, task int) (models.ConductorTask, error) {
	var t models.ConductorTask
	err := r.DB.QueryRow(`SELECT `+conductorColumns+` FROM conductor_task WHERE project=? AND task=?`, project, task).Scan(&t.TaskID, &t.ProjectID, &t.State, &t.JobID, &t.ExpectedStage, &t.Message, &t.UpdatedAt, &t.SelectionKey)
	return t, err
}

// Retry is idempotent while this exact managed task already has queued work.
func (r *ConductorRepo) StartedTask(project, task int) (*models.AgentJob, error) {
	t, err := r.Task(project, task)
	if err != nil {
		return nil, err
	}
	if t.State != "queued" && t.State != "working" {
		return nil, nil
	}
	j, err := (&AgentJobRepo{DB: r.DB}).FindOne(t.JobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConductorConflict
	}
	return j, err
}
