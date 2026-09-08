package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

type AgentRunRepo struct {
	DB *sql.DB
}

const agentRunColumns = "id, job, COALESCE(output, ''), COALESCE(summary, ''), exitCode, COALESCE(usageJson, ''), createdAt, artifactJson"

func scanAgentRun(scanner interface{ Scan(...any) error }, run *models.AgentRun) error {
	var exitCode sql.NullInt64
	var usageJson string
	var createdAt string
	var artifacts string
	var output string
	var summary string
	if err := scanner.Scan(
		&run.ID,
		&run.Job,
		&output,
		&summary,
		&exitCode,
		&usageJson,
		&createdAt, &artifacts,
	); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(artifacts), &run.Artifacts); err != nil {
		return err
	}
	run.Output = output
	run.Summary = summary
	if exitCode.Valid {
		v := int(exitCode.Int64)
		run.ExitCode = &v
	} else {
		run.ExitCode = nil
	}
	run.UsageJson = usageJson // Legacy provider output remains readable.
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return fmt.Errorf("parse agent_run %d createdAt: %w", run.ID, err)
	}
	run.CreatedAt = &parsed
	return nil
}

// CreateRun inserts a new agent_run row.
func (repo *AgentRunRepo) CreateRun(run *models.AgentRun) (*models.AgentRun, error) {
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

// ListByJob returns all runs for a given job ordered by createdAt.
func (repo *AgentRunRepo) ListByJob(jobID int) ([]models.AgentRun, error) {
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
