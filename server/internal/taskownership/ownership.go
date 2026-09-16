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
