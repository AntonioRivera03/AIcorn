// Package taskownership centralizes the reservation shared by AI entry points.
// Call Check inside the same write transaction as the protected mutation.
package taskownership

import (
	"database/sql"
	"errors"
	"fmt"
)

var ErrBusy = errors.New("task is owned by another agent")
var ErrNotOwner = errors.New("this agent run no longer owns the task")

type Owner struct {
	TaskID int    `json:"taskId"`
	Kind   string `json:"kind"`
	JobID  int    `json:"jobId,omitempty"`
	Name   string `json:"name"`
	State  string `json:"state"`
}
type Querier interface{ QueryRow(string, ...any) *sql.Row }

func Current(q Querier, task int) (*Owner, error) {
	o := Owner{TaskID: task}
	err := q.QueryRow(`SELECT 'conductor',COALESCE(job,0),'Conductor',state FROM conductor_task WHERE task=? AND state IN ('waiting','planning','queued','working')`, task).Scan(&o.Kind, &o.JobID, &o.Name, &o.State)
	if err == nil {
		return &o, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	err = q.QueryRow(`SELECT 'agent',id,COALESCE(NULLIF(json_extract(requestJson,'$.presetName'),''),'Task agent'),status FROM agent_job WHERE task=? AND status IN ('pending','claimed','running','canceling') LIMIT 1`, task).Scan(&o.Kind, &o.JobID, &o.Name, &o.State)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &o, err
}

func Begin(db *sql.DB) (*sql.Tx, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	// A write statement obtains SQLite's writer lock even when it matches no rows.
	if _, err = tx.Exec("UPDATE task SET id=id WHERE id=-1"); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func Check(q Querier, task, job int) error {
	var exists int
	if err := q.QueryRow("SELECT id FROM task WHERE id=?", task).Scan(&exists); err != nil {
		return err
	}
	if job > 0 {
		var active bool
		if err := q.QueryRow("SELECT EXISTS(SELECT 1 FROM agent_job WHERE id=? AND task=? AND status IN ('claimed','running'))", job, task).Scan(&active); err != nil {
			return err
		}
		if !active {
			return ErrNotOwner
		}
	}
	owner, err := Current(q, task)
	if err != nil {
		return err
	}
	if owner != nil && (job == 0 || owner.JobID != job) {
		return fmt.Errorf("%w: #%d is managed by %s (%s); wait until it finishes", ErrBusy, task, owner.Name, owner.State)
	}
	return nil
}

// ListProject is shared by every UI in a project, avoiding a poll per task.
func ListProject(db *sql.DB, project int) ([]Owner, error) {
	var exists int
	if err := db.QueryRow("SELECT id FROM project WHERE id=?", project).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT ct.task,'conductor',COALESCE(ct.job,0),'Conductor',ct.state
 FROM conductor_task ct JOIN task t ON t.id=ct.task JOIN checklist c ON c.id=t.checklist
 WHERE c.project=? AND ct.state IN ('waiting','planning','queued','working')
 UNION ALL
 SELECT j.task,'agent',j.id,COALESCE(NULLIF(json_extract(j.requestJson,'$.presetName'),''),'Task agent'),j.status
 FROM agent_job j JOIN task t ON t.id=j.task JOIN checklist c ON c.id=t.checklist
 WHERE c.project=? AND j.status IN ('pending','claimed','running','canceling')
 AND NOT EXISTS(SELECT 1 FROM conductor_task ct WHERE ct.task=j.task AND ct.state IN ('waiting','planning','queued','working'))`, project, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owners := []Owner{}
	for rows.Next() {
		var owner Owner
		if err = rows.Scan(&owner.TaskID, &owner.Kind, &owner.JobID, &owner.Name, &owner.State); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
	}
	return owners, rows.Err()
}
