package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

var ErrConductorConflict = errors.New("Conductor state changed; refresh and try again")
var ErrConductorPaused = errors.New("Conductor is paused")
var ErrConductorConfig = errors.New("Conductor needs three distinct, non-done stages from this project's workflow")
var ErrConductorBlocked = errors.New("An unresolved blocking task must be completed before this task can start; resolve it, then recheck")

type ConductorRepo struct{ DB *sql.DB }

// Acquire SQLite's write lock before reading a transition's preconditions. This
// avoids a deferred read transaction becoming an un-upgradable stale snapshot
// when a user edits another task between validation and the first write.
func (r *ConductorRepo) beginTransition(project int) (*sql.Tx, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE conductor_project SET enabled=enabled WHERE project=?`, project); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// A human moving reviewed work onward takes ownership. Keep its run history,
// but stop counting the task as waiting for review or automatically re-running it.
func (r *ConductorRepo) ReleaseReviewed() error {
	_, err := r.DB.Exec(`DELETE FROM conductor_task WHERE state='completed' AND EXISTS(
 SELECT 1 FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=conductor_task.task AND (t.stage<>conductor_task.expectedStage OR c.project<>conductor_task.project))`)
	return err
}

// ClaimNext shares the existing queue but leaves paused Conductor work pending.
func (r *ConductorRepo) ClaimNext() (*models.AgentJob, error) {
	var job models.AgentJob
	err := scanAgentJob(r.DB.QueryRow(`UPDATE agent_job SET status='claimed',claimedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now'),attempts=attempts+1
 WHERE id=(SELECT j.id FROM agent_job j WHERE j.status='pending' AND
 (json_type(j.requestJson,'$.conductor') IS NULL OR EXISTS(SELECT 1 FROM conductor_task ct JOIN conductor_project cp ON cp.project=ct.project WHERE ct.job=j.id AND cp.enabled=1 AND ct.state IN ('planning','queued')))
 ORDER BY j.createdAt,j.id LIMIT 1) RETURNING `+agentJobColumns), &job)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &job, err
}

func (r *ConductorRepo) Settings(project int) (models.ConductorSettings, string, error) {
	s := models.DefaultConductorSettings()
	var raw string
	var enabled bool
	err := r.DB.QueryRow(`SELECT cp.settings, cp.enabled FROM conductor_project cp WHERE cp.project=?`, project).Scan(&raw, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		err = r.DB.QueryRow(`SELECT id FROM project WHERE id=?`, project).Scan(&exists)
		return s, "", err
	}
	if err != nil {
		return s, "", err
	}
	if err = json.Unmarshal([]byte(raw), &s); err != nil {
		return s, "", err
	}
	s.Enabled = enabled
	return s, raw, nil
}

// SaveSettings compares the previous document so concurrent settings edits cannot
// silently overwrite one another. The enabled column also gates queue claims.
func (r *ConductorRepo) SaveSettings(project int, s models.ConductorSettings, previous string) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var res sql.Result
	if previous == "" {
		res, err = r.DB.Exec(`INSERT INTO conductor_project(project,enabled,settings) VALUES(?,?,?) ON CONFLICT(project) DO NOTHING`, project, s.Enabled, string(raw))
	} else {
		res, err = r.DB.Exec(`UPDATE conductor_project SET enabled=?,settings=? WHERE project=? AND settings=?`, s.Enabled, string(raw), project, previous)
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrConductorConflict
	}
	return err
}

type conductorQuerier interface{ QueryRow(string, ...any) *sql.Row }

func validateConductorStages(q conductorQuerier, project int, s models.ConductorSettings) error {
	if s.PlanningStage <= 0 || s.WorkingStage <= 0 || s.CompletionStage <= 0 || s.PlanningStage == s.WorkingStage || s.PlanningStage == s.CompletionStage || s.WorkingStage == s.CompletionStage {
		return ErrConductorConfig
	}
	var count int
	if err := q.QueryRow(`SELECT COUNT(*) FROM stage s JOIN project p ON p.workflow=s.workflow WHERE p.id=? AND s.id IN (?,?,?) AND s.type<>'done'`, project, s.PlanningStage, s.WorkingStage, s.CompletionStage).Scan(&count); err != nil {
		return err
	}
	if count != 3 {
		return ErrConductorConfig
	}
	return nil
}

func validateConductorBlockers(q conductorQuerier, task int) error {
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM task_relationship r JOIN task_relationship_type rt ON rt.id=r.relationshipType JOIN task t ON t.id=r.fromTask JOIN stage s ON s.id=t.stage WHERE r.toTask=? AND rt.behavior='blocking' AND s.type<>'done'`, task).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrConductorBlocked
	}
	return nil
}

func (r *ConductorRepo) ValidateStages(project int, s models.ConductorSettings) error {
	return validateConductorStages(r.DB, project, s)
}

const conductorColumns = `task,project,state,COALESCE(job,0),expectedStage,message,updatedAt`

func (r *ConductorRepo) Tasks(project int) ([]models.ConductorTask, error) {
	where := ""
	var args []any
	if project > 0 {
		where = " WHERE project=?"
		args = append(args, project)
	}
	rows, err := r.DB.Query(`SELECT `+conductorColumns+` FROM conductor_task`+where+` ORDER BY updatedAt,task`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.ConductorTask{}
	for rows.Next() {
		var t models.ConductorTask
		if err := rows.Scan(&t.TaskID, &t.ProjectID, &t.State, &t.JobID, &t.ExpectedStage, &t.Message, &t.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// Manage is one atomic bulk operation. All IDs reach the server; invalid,
// cross-project, completed and already-active tasks are counted as skipped.
func (r *ConductorRepo) Manage(project int, ids []int, action string) (models.BulkResult, error) {
	result := models.BulkResult{}
	unique := map[int]bool{}
	args := []any{project}
	marks := []string{}
	for _, id := range ids {
		if !unique[id] {
			unique[id] = true
			args = append(args, id)
			marks = append(marks, "?")
		}
	}
	if len(marks) == 0 {
		return result, nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var res sql.Result
	if action == "release" {
		// Cancel only the run owned by Conductor, including the claim/start race.
		_, err = tx.Exec(`UPDATE agent_job SET status=CASE WHEN status IN ('pending','claimed') THEN 'canceled' ELSE 'canceling' END,
 finishedAt=CASE WHEN status IN ('pending','claimed') THEN strftime('%Y-%m-%dT%H:%M:%SZ','now') ELSE finishedAt END
 WHERE status IN ('pending','claimed','running') AND id IN (SELECT job FROM conductor_task WHERE project=? AND task IN (`+strings.Join(marks, ",")+`))`, args...)
		if err != nil {
			return result, err
		}
		res, err = tx.Exec(`DELETE FROM conductor_task WHERE project=? AND task IN (`+strings.Join(marks, ",")+`)`, args...)
	} else if action == "send" || action == "recheck" {
		conflict := "DO NOTHING"
		if action == "recheck" {
			conflict = `DO UPDATE SET state='waiting',job=NULL,expectedStage=excluded.expectedStage,message='',updatedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE conductor_task.state IN ('needs_context','failed','held')`
		}
		res, err = tx.Exec(`INSERT INTO conductor_task(task,project,expectedStage)
 SELECT t.id,c.project,t.stage FROM task t JOIN checklist c ON c.id=t.checklist JOIN stage s ON s.id=t.stage
 WHERE c.project=? AND t.id IN (`+strings.Join(marks, ",")+`) AND s.type<>'done'
 AND NOT EXISTS(SELECT 1 FROM agent_job j WHERE j.task=t.id AND j.status IN ('pending','claimed','running','canceling'))
 ON CONFLICT(task) `+conflict, args...)
	} else {
		return result, fmt.Errorf("unknown Conductor action")
	}
	if err != nil {
		return result, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return result, err
	}
	result.Success = int(n)
	result.Skipped = len(unique) - int(n)
	return result, tx.Commit()
}

func (r *ConductorRepo) SetState(t models.ConductorTask, state, message string) error {
	_, err := r.DB.Exec(`UPDATE conductor_task SET state=?,message=?,updatedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE task=? AND state=? AND COALESCE(job,0)=?`, state, message, t.TaskID, t.State, t.JobID)
	return err
}

// Advance commits body, stage, assignment, queue insertion and the Conductor
// cursor together. Exact task/ownership comparisons protect concurrent edits.
func (r *ConductorRepo) Advance(t models.ConductorTask, current *models.TaskWithProject, body string, stage int, state, message string, request *models.AIRunRequest, contract models.ConductorSettings) error {
	tx, err := r.beginTransition(t.ProjectID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = validateConductorStages(tx, t.ProjectID, contract); err != nil {
		return err
	}
	if request != nil {
		if request.Conductor.Phase == "working" {
			if err = validateConductorBlockers(tx, t.TaskID); err != nil {
				return err
			}
		}
		var enabled bool
		if err := tx.QueryRow(`SELECT enabled FROM conductor_project WHERE project=?`, t.ProjectID).Scan(&enabled); err != nil {
			return err
		}
		if !enabled {
			return ErrConductorPaused
		}
	}
	var owned int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM conductor_task WHERE task=? AND project=? AND state=? AND COALESCE(job,0)=?`, t.TaskID, t.ProjectID, t.State, t.JobID).Scan(&owned); err != nil {
		return err
	}
	if owned != 1 || current.ProjectID != t.ProjectID || current.Stage != t.ExpectedStage {
		return ErrConductorConflict
	}
	assignee := current.Assignee
	if request != nil && request.Conductor.Phase == "working" {
		assignee = "AI · " + request.PresetName
		if request.PresetName == "" {
			assignee = "AI · " + request.Model
		}
	}
	res, err := tx.Exec(`UPDATE task SET body=?,stage=?,assignee=? WHERE id=? AND stage=? AND COALESCE(body,'')=? AND COALESCE(name,'')=? AND COALESCE(assignee,'')=? AND checklist=?`, body, stage, assignee, t.TaskID, current.Stage, current.Body, current.Name, current.Assignee, current.Checklist)
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
	job := t.JobID
	if request != nil {
		raw, err := json.Marshal(request)
		if err != nil {
			return err
		}
		if err = tx.QueryRow(`INSERT INTO agent_job(task,status,requestJson,persona) VALUES(?,'pending',?,(SELECT id FROM persona WHERE id=?)) RETURNING id`, t.TaskID, string(raw), request.AgentID).Scan(&job); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed: agent_job.task") {
				return ErrActiveAIRun
			}
			return err
		}
	}
	_, err = tx.Exec(`UPDATE conductor_task SET state=?,job=?,expectedStage=?,message=?,updatedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE task=?`, state, nullableConductorJob(job), stage, message, t.TaskID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func nullableConductorJob(job int) any {
	if job == 0 {
		return nil
	}
	return job
}

// BeginJob moves an implementation to Doing only when the queue starts it.
// A pause between claim and start returns the job to pending without execution.
func (r *ConductorRepo) BeginJob(job *models.AgentJob) (bool, error) {
	tx, err := r.beginTransition(job.Request.ProjectID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var enabled bool
	var expectedStage int
	var state, body, name string
	err = tx.QueryRow(`SELECT cp.enabled,ct.expectedStage,ct.state,COALESCE(t.body,''),COALESCE(t.name,'') FROM conductor_task ct JOIN conductor_project cp ON cp.project=ct.project JOIN task t ON t.id=ct.task JOIN checklist c ON c.id=t.checklist WHERE ct.task=? AND ct.job=? AND c.project=ct.project AND t.stage=ct.expectedStage`, job.Task, job.ID).Scan(&enabled, &expectedStage, &state, &body, &name)
	if err != nil {
		return false, err
	}
	c := job.Request.Conductor
	if c.Phase == "working" {
		if err = validateConductorBlockers(tx, job.Task); err != nil {
			return false, err
		}
	}
	if err = validateConductorStages(tx, job.Request.ProjectID, c.Settings); err != nil {
		return false, err
	}
	if body != c.SourceBody || name != job.Request.TaskName {
		return false, fmt.Errorf("%w: task content changed before the agent started; recheck to use the latest context", ErrConductorConflict)
	}
	if !enabled {
		_, err = tx.Exec(`UPDATE agent_job SET status='pending',claimedAt=NULL,attempts=MAX(0,attempts-1) WHERE id=? AND status='claimed'`, job.ID)
		if err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	if (c.Phase == "planning" && state != "planning") || (c.Phase == "working" && state != "queued") {
		return false, ErrConductorConflict
	}
	res, err := tx.Exec(`UPDATE agent_job SET status='running',startedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=? AND status='claimed'`, job.ID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if c.Phase == "working" {
		_, err = tx.Exec(`UPDATE task SET stage=? WHERE id=?`, c.Settings.WorkingStage, job.Task)
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(`UPDATE conductor_task SET state='working',expectedStage=?,message='Agent is working',updatedAt=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE task=?`, c.Settings.WorkingStage, job.Task)
		if err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (r *ConductorRepo) OwnsJob(job *models.AgentJob) (bool, error) {
	var count int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM conductor_task ct JOIN task t ON t.id=ct.task JOIN checklist c ON c.id=t.checklist WHERE ct.job=? AND ct.task=? AND ct.state IN ('planning','working') AND t.stage=ct.expectedStage AND c.project=ct.project`, job.ID, job.Task).Scan(&count)
	return count == 1, err
}
