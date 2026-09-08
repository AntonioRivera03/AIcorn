package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

// ErrJobStatusConflict is returned when a Complete transaction finds the job
// is not in claimed/running status. Callers map this to ErrInvalidJobStatus.
var ErrJobStatusConflict = errors.New("agent job status does not allow this transition")

type AgentJobRepo struct {
	DB *sql.DB
}

const agentJobColumns = "id, task, COALESCE(persona,0), status, fromStage, toStage, claimedAt, startedAt, finishedAt, attempts, COALESCE(error, ''), createdAt, requestJson, progress"

func scanAgentJob(scanner interface{ Scan(...any) error }, job *models.AgentJob) error {
	var fromStage sql.NullInt64
	var toStage sql.NullInt64
	var claimedAt sql.NullString
	var startedAt sql.NullString
	var finishedAt sql.NullString
	var createdAt string
	var request string
	if err := scanner.Scan(
		&job.ID,
		&job.Task,
		&job.Persona,
		&job.Status,
		&fromStage,
		&toStage,
		&claimedAt,
		&startedAt,
		&finishedAt,
		&job.Attempts,
		&job.Error,
		&createdAt,
		&request, &job.Progress,
	); err != nil {
		return err
	}
	if request != "" && request != "{}" {
		if err := json.Unmarshal([]byte(request), &job.Request); err != nil {
			return err
		}
	}
	if fromStage.Valid {
		v := int(fromStage.Int64)
		job.FromStage = &v
	} else {
		job.FromStage = nil
	}
	if toStage.Valid {
		v := int(toStage.Int64)
		job.ToStage = &v
	} else {
		job.ToStage = nil
	}
	if claimedAt.Valid && claimedAt.String != "" {
		t, err := time.Parse(time.RFC3339, claimedAt.String)
		if err != nil {
			return fmt.Errorf("parse agent_job %d claimedAt: %w", job.ID, err)
		}
		job.ClaimedAt = &t
	} else {
		job.ClaimedAt = nil
	}
	if startedAt.Valid && startedAt.String != "" {
		t, err := time.Parse(time.RFC3339, startedAt.String)
		if err != nil {
			return fmt.Errorf("parse agent_job %d startedAt: %w", job.ID, err)
		}
		job.StartedAt = &t
	} else {
		job.StartedAt = nil
	}
	if finishedAt.Valid && finishedAt.String != "" {
		t, err := time.Parse(time.RFC3339, finishedAt.String)
		if err != nil {
			return fmt.Errorf("parse agent_job %d finishedAt: %w", job.ID, err)
		}
		job.FinishedAt = &t
	} else {
		job.FinishedAt = nil
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return fmt.Errorf("parse agent_job %d createdAt: %w", job.ID, err)
	}
	job.CreatedAt = &parsed
	return nil
}

func nullableIntArg(v *int) any {
	if v != nil {
		return *v
	}
	return nil
}

func nullableTimeArg(t *time.Time) any {
	if t != nil {
		return t.Format(time.RFC3339)
	}
	return nil
}

// Create inserts a new agent_job row and returns the inserted row.
func (repo *AgentJobRepo) Create(job *models.AgentJob) (*models.AgentJob, error) {
	query := `
		INSERT INTO agent_job (task, persona, status, fromStage, toStage, claimedAt, startedAt, finishedAt, attempts, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING ` + agentJobColumns + `;
	`
	created := models.AgentJob{}
	err := scanAgentJob(repo.DB.QueryRow(query,
		job.Task,
		job.Persona,
		job.Status,
		nullableIntArg(job.FromStage),
		nullableIntArg(job.ToStage),
		nullableTimeArg(job.ClaimedAt),
		nullableTimeArg(job.StartedAt),
		nullableTimeArg(job.FinishedAt),
		job.Attempts,
		job.Error,
	), &created)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// FindOne returns the agent_job with the given id or sql.ErrNoRows.
func (repo *AgentJobRepo) FindOne(id int) (*models.AgentJob, error) {
	job := models.AgentJob{}
	if err := scanAgentJob(repo.DB.QueryRow("SELECT "+agentJobColumns+" FROM agent_job WHERE id = ?;", id), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (repo *AgentJobRepo) FindPendingByTask(taskID int) (*models.AgentJob, error) {
	job := models.AgentJob{}
	if err := scanAgentJob(repo.DB.QueryRow("SELECT "+agentJobColumns+" FROM agent_job WHERE task = ? AND status = 'pending' ORDER BY createdAt ASC LIMIT 1;", taskID), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// FindByTask returns all jobs for a given task ordered by createdAt.
func (repo *AgentJobRepo) FindByTask(taskID int) ([]models.AgentJob, error) {
	rows, err := repo.DB.Query("SELECT "+agentJobColumns+" FROM agent_job WHERE task = ? ORDER BY createdAt ASC;", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var j models.AgentJob
		if err := scanAgentJob(rows, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ListByStatus returns all jobs with the given status ordered by createdAt.
func (repo *AgentJobRepo) ListByStatus(status string) ([]models.AgentJob, error) {
	rows, err := repo.DB.Query("SELECT "+agentJobColumns+" FROM agent_job WHERE status = ? ORDER BY createdAt ASC;", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var j models.AgentJob
		if err := scanAgentJob(rows, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ClaimNext atomically claims the oldest pending job and returns it.
// Returns sql.ErrNoRows when no pending job exists.
func (repo *AgentJobRepo) ClaimNext() (*models.AgentJob, error) {
	query := `
		UPDATE agent_job
		SET status = 'claimed',
		    claimedAt = strftime('%Y-%m-%dT%H:%M:%SZ','now'),
		    attempts = attempts + 1
		WHERE id = (SELECT id FROM agent_job WHERE status = 'pending' ORDER BY createdAt LIMIT 1)
		RETURNING ` + agentJobColumns + `;
	`
	job := models.AgentJob{}
	if err := scanAgentJob(repo.DB.QueryRow(query), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// ResetStale moves stale claimed and running jobs older than timeout back to pending.
// A job is stale if it is claimed with claimedAt < cutoff OR running with startedAt < cutoff.
// Both claimedAt and startedAt are cleared to NULL so the next ClaimNext is clean.
// Returns the number of rows reset.
func (repo *AgentJobRepo) ResetStale(timeout time.Duration) (int, error) {
	cutoff := time.Now().Add(-timeout).Format(time.RFC3339)
	res, err := repo.DB.Exec(`
		UPDATE agent_job
		SET status = 'pending',
		    claimedAt = NULL,
		    startedAt = NULL
		WHERE (status = 'claimed' AND claimedAt < ?)
		   OR (status = 'running' AND startedAt < ?);
	`, cutoff, cutoff)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}

// UpdateStatus sets the status of a job.
func (repo *AgentJobRepo) UpdateStatus(id int, status string) (bool, error) {
	res, err := repo.DB.Exec("UPDATE agent_job SET status = ? WHERE id = ?;", status, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// MarkRunning transitions a claimed job to running and stamps startedAt.
// Returns false if the job was not in claimed state.
func (repo *AgentJobRepo) MarkRunning(id int) (bool, error) {
	res, err := repo.DB.Exec(`
		UPDATE agent_job
		SET status = 'running',
		    startedAt = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id = ? AND status = 'claimed';
	`, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// MarkFinished transitions a job to a terminal status (completed/failed)
// and stamps finishedAt. Any current status is accepted — the service
// layer enforces allowed transitions.
func (repo *AgentJobRepo) MarkFinished(id int, status string) (bool, error) {
	res, err := repo.DB.Exec(`
		UPDATE agent_job
		SET status = ?,
		    finishedAt = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id = ?;
	`, status, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// Fail marks a job as failed and records an error message alongside finishedAt.
func (repo *AgentJobRepo) Fail(id int, errMsg string) (bool, error) {
	res, err := repo.DB.Exec(`
		UPDATE agent_job
		SET status = 'failed',
		    error = ?,
		    finishedAt = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id = ?;
	`, errMsg, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// CreateRun inserts an agent_run row. Kept on AgentJobRepo for convenience
// as specified in the task description; see also AgentRunRepo.
func (repo *AgentJobRepo) CreateRun(run *models.AgentRun) (*models.AgentRun, error) {
	query := `
		INSERT INTO agent_run (job, output, summary, exitCode, usageJson)
		VALUES (?, ?, ?, ?, ?)
		RETURNING ` + agentRunColumns + `;
	`
	created := models.AgentRun{}
	if err := scanAgentRun(repo.DB.QueryRow(query,
		run.Job,
		run.Output,
		run.Summary,
		nullableIntArg(run.ExitCode),
		run.UsageJson,
	), &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (repo *AgentJobRepo) ListByJob(jobID int) ([]models.AgentRun, error) {
	rows, err := repo.DB.Query("SELECT "+agentRunColumns+" FROM agent_run WHERE job = ? ORDER BY createdAt ASC;", jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []models.AgentRun{}
	for rows.Next() {
		var r models.AgentRun
		if err := scanAgentRun(rows, &r); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (repo *AgentJobRepo) ListAll() ([]models.AgentJob, error) {
	rows, err := repo.DB.Query("SELECT " + agentJobColumns + " FROM agent_job ORDER BY createdAt ASC;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var j models.AgentJob
		if err := scanAgentJob(rows, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (repo *AgentJobRepo) BulkEnqueueForStageTx(tx *sql.Tx, taskIDs []int, personaID int, toStage int) error {
	if len(taskIDs) == 0 {
		return nil
	}
	placeholders, args := intIdPlaceholders(taskIDs)
	query := `INSERT INTO agent_job (task, persona, status, fromStage, toStage)
		SELECT id, ?, 'pending', NULL, ? FROM task WHERE id IN (` + placeholders + `)`
	allArgs := make([]any, 0, 2+len(args))
	allArgs = append(allArgs, personaID, toStage)
	allArgs = append(allArgs, args...)
	_, err := tx.Exec(query, allArgs...)
	return err
}

func (repo *AgentJobRepo) CreateJobsTx(tx *sql.Tx, jobs []models.AgentJob) error {
	if len(jobs) == 0 {
		return nil
	}
	vals := []string{}
	args := []any{}
	for _, j := range jobs {
		vals = append(vals, "(?, ?, ?, ?, ?)")
		args = append(args, j.Task, j.Persona, j.Status, nullableIntArg(j.FromStage), nullableIntArg(j.ToStage))
	}
	query := `INSERT INTO agent_job (task, persona, status, fromStage, toStage) VALUES ` + strings.Join(vals, ", ")
	_, err := tx.Exec(query, args...)
	return err
}

func (repo *AgentJobRepo) ListActiveByProject(projectID int) ([]models.AgentJob, error) {
	return repo.ListFiltered(&projectID, []string{
		models.AgentJobStatusPending,
		models.AgentJobStatusClaimed,
		models.AgentJobStatusRunning,
	})
}

func (repo *AgentJobRepo) ListActiveByProjectIDs(projectIDs []int) ([]models.AgentJob, error) {
	if len(projectIDs) == 0 {
		return []models.AgentJob{}, nil
	}
	placeholders, args := intIdPlaceholders(projectIDs)
	query := `
		SELECT aj.id, aj.task, COALESCE(aj.persona,0), aj.status, aj.fromStage, aj.toStage, aj.claimedAt, aj.startedAt, aj.finishedAt, aj.attempts, COALESCE(aj.error, ''), aj.createdAt, aj.requestJson, aj.progress
		FROM agent_job aj
		JOIN task t ON t.id = aj.task
		JOIN checklist c ON c.id = t.checklist
		WHERE c.project IN (` + placeholders + `)
		  AND aj.status IN ('pending','claimed','running','canceling')
		ORDER BY aj.createdAt ASC;`
	rows, err := repo.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var j models.AgentJob
		if err := scanAgentJob(rows, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (repo *AgentJobRepo) ListFiltered(projectID *int, statuses []string) ([]models.AgentJob, error) {
	expanded := []string{}
	for _, s := range statuses {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		if trimmed == "active" {
			expanded = append(expanded, models.AgentJobStatusPending, models.AgentJobStatusClaimed, models.AgentJobStatusRunning, "canceling")
		} else {
			expanded = append(expanded, trimmed)
		}
	}
	seen := map[string]bool{}
	deduped := []string{}
	for _, s := range expanded {
		if !seen[s] {
			seen[s] = true
			deduped = append(deduped, s)
		}
	}
	expanded = deduped

	hasProject := projectID != nil
	hasStatuses := len(expanded) > 0

	if !hasProject && !hasStatuses {
		return repo.ListAll()
	}
	if hasProject && hasStatuses {
		placeholders, args := stringPlaceholders(expanded)
		query := `
			SELECT aj.id, aj.task, COALESCE(aj.persona,0), aj.status, aj.fromStage, aj.toStage, aj.claimedAt, aj.startedAt, aj.finishedAt, aj.attempts, COALESCE(aj.error, ''), aj.createdAt, aj.requestJson, aj.progress
			FROM agent_job aj
			JOIN task t ON t.id = aj.task
			JOIN checklist c ON c.id = t.checklist
			WHERE c.project = ?
			  AND aj.status IN (` + placeholders + `)
			ORDER BY aj.createdAt ASC;`
		allArgs := append([]any{*projectID}, args...)
		rows, err := repo.DB.Query(query, allArgs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		jobs := []models.AgentJob{}
		for rows.Next() {
			var j models.AgentJob
			if err := scanAgentJob(rows, &j); err != nil {
				return nil, err
			}
			jobs = append(jobs, j)
		}
		return jobs, rows.Err()
	}
	if hasProject {
		query := `
			SELECT aj.id, aj.task, COALESCE(aj.persona,0), aj.status, aj.fromStage, aj.toStage, aj.claimedAt, aj.startedAt, aj.finishedAt, aj.attempts, COALESCE(aj.error, ''), aj.createdAt, aj.requestJson, aj.progress
			FROM agent_job aj
			JOIN task t ON t.id = aj.task
			JOIN checklist c ON c.id = t.checklist
			WHERE c.project = ?
			ORDER BY aj.createdAt ASC;`
		rows, err := repo.DB.Query(query, *projectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		jobs := []models.AgentJob{}
		for rows.Next() {
			var j models.AgentJob
			if err := scanAgentJob(rows, &j); err != nil {
				return nil, err
			}
			jobs = append(jobs, j)
		}
		return jobs, rows.Err()
	}
	placeholders, args := stringPlaceholders(expanded)
	query := `SELECT ` + agentJobColumns + ` FROM agent_job WHERE status IN (` + placeholders + `) ORDER BY createdAt ASC;`
	rows, err := repo.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []models.AgentJob{}
	for rows.Next() {
		var j models.AgentJob
		if err := scanAgentJob(rows, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func stringPlaceholders(vals []string) (string, []any) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(vals)), ",")
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	return placeholders, args
}

func (repo *AgentJobRepo) CreateRunAndCompleteTx(run *models.AgentRun, finalStatus string) error {
	tx, err := repo.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO agent_run (job, output, summary, exitCode, usageJson)
		VALUES (?, ?, ?, ?, ?);
	`, run.Job, run.Output, run.Summary, nullableIntArg(run.ExitCode), run.UsageJson)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`
		UPDATE agent_job
		SET status = ?,
		    finishedAt = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id = ? AND status IN ('claimed','running');
	`, finalStatus, run.Job)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrJobStatusConflict
	}
	return tx.Commit()
}
