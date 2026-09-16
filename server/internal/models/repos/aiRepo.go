package repos

import (
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"strings"
)

var ErrActiveAIRun = taskownership.ErrBusy

func (r *AgentJobRepo) AISettings() (models.AISettings, error) {
	var s models.AISettings
	err := r.DB.QueryRow("SELECT model, executable, timeoutSeconds FROM ai_settings WHERE id=1").Scan(&s.Model, &s.Executable, &s.TimeoutSeconds)
	return s, err
}
func (r *AgentJobRepo) UpdateAISettings(s models.AISettings) error {
	_, err := r.DB.Exec("UPDATE ai_settings SET model=?, executable=?, timeoutSeconds=? WHERE id=1", s.Model, s.Executable, s.TimeoutSeconds)
	return err
}
func (r *AgentJobRepo) EnqueueAI(taskID, presetID int, request models.AIRunRequest, chatTurn ...int) (*models.AgentJob, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var preset any
	if presetID > 0 {
		preset = presetID
	}
	var job models.AgentJob
	tx, err := taskownership.Begin(r.DB)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = taskownership.Check(tx, taskID, 0, chatTurn...); err != nil {
		return nil, err
	}
	// The project may have changed while the immutable snapshot was prepared.
	if request.ProjectID > 0 {
		var matches bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND c.project=?)", taskID, request.ProjectID).Scan(&matches); err != nil {
			return nil, err
		}
		if !matches {
			return nil, ErrConductorConflict
		}
	}
	err = scanAgentJob(tx.QueryRow(`INSERT INTO agent_job(task,persona,status,requestJson) VALUES(?,?,'pending',?) RETURNING `+agentJobColumns, taskID, preset, string(raw)), &job)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: agent_job.task") {
		return nil, ErrActiveAIRun
	}
	if err != nil {
		return nil, err
	}
	return &job, tx.Commit()
}
func (r *AgentJobRepo) SetProgress(id int, text string) error {
	_, err := r.DB.Exec("UPDATE agent_job SET progress=? WHERE id=? AND status IN ('claimed','running')", text, id)
	return err
}
func (r *AgentJobRepo) CancelAI(id int) (bool, error) {
	res, err := r.DB.Exec(`UPDATE agent_job SET status=CASE WHEN status IN ('pending','claimed') THEN 'canceled' ELSE 'canceling' END,
 finishedAt=CASE WHEN status IN ('pending','claimed') THEN strftime('%Y-%m-%dT%H:%M:%SZ','now') ELSE finishedAt END
 WHERE id=? AND status IN ('pending','claimed','running')`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// A server restart never silently replays an uncertain execution.
func (r *AgentJobRepo) InterruptInFlight() error {
	_, err := r.DB.Exec(`UPDATE agent_job SET status='interrupted', error='Server stopped during this run. Partial workspace may be available.',
 finishedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE status IN ('claimed','running','canceling')`)
	return err
}

// Persist a checkpoint before starting the process, so a crash cannot hide its workspace.
func (r *AgentJobRepo) Checkpoint(id int, output, usage string, artifacts models.AIRunArtifacts) error {
	raw, err := json.Marshal(artifacts)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(`UPDATE agent_run SET output=?, usageJson=?, artifactJson=? WHERE job=?`, output, usage, string(raw), id)
	return err
}
func (r *AgentJobRepo) BeginAttempt(id int) error {
	_, err := r.DB.Exec(`INSERT INTO agent_run(job,output) SELECT id,'' FROM agent_job WHERE id=? AND status IN ('running','canceling') AND NOT EXISTS(SELECT 1 FROM agent_run WHERE job=?)`, id, id)
	return err
}
func (r *AgentJobRepo) FinishAI(id int, status, message, output, usage string, exit int, artifacts models.AIRunArtifacts) error {
	raw, err := json.Marshal(artifacts)
	if err != nil {
		return err
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A cancel request wins the completion race and retains whatever was produced.
	res, err := tx.Exec(`UPDATE agent_job SET status=CASE WHEN status='canceling' THEN 'canceled' ELSE ? END,error=?,progress='',finishedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=? AND status IN ('claimed','running','canceling')`, status, message, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrJobStatusConflict
	}
	_, err = tx.Exec(`UPDATE agent_run SET output=?,summary=?,exitCode=?,usageJson=?,artifactJson=? WHERE job=?`, output, message, exit, usage, string(raw), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *AgentJobRepo) LatestByProject(projectID int) ([]models.AgentJob, error) {
	rows, err := r.DB.Query(`SELECT id,task,COALESCE(persona,0),status,fromStage,toStage,claimedAt,startedAt,finishedAt,attempts,COALESCE(error,''),createdAt,'{}',progress
 FROM agent_job WHERE id IN (SELECT MAX(j.id) FROM agent_job j JOIN task t ON t.id=j.task JOIN checklist c ON c.id=t.checklist WHERE c.project=? GROUP BY j.task)`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var job models.AgentJob
		if err := scanAgentJob(rows, &job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
