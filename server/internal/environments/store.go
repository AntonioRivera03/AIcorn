package environments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

type Store struct{ DB *sql.DB }

func (s *Store) Installation() (string, error) {
	var token string
	err := s.DB.QueryRow(`SELECT token FROM environment_installation WHERE id=1`).Scan(&token)
	return token, err
}
func (s *Store) Settings(project int) (Settings, error) {
	settings := Defaults()
	var raw string
	err := s.DB.QueryRow(`SELECT COALESCE(e.settings,'{}') FROM project p LEFT JOIN environment_settings e ON e.project=p.id WHERE p.id=?`, project).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &settings)
	}
	return settings, err
}
func (s *Store) UpdateSettings(project int, patch map[string]json.RawMessage) (Settings, error) {
	current, err := s.Settings(project)
	if err != nil {
		return current, err
	}
	data, _ := json.Marshal(current)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	for key, value := range patch {
		if _, ok := fields[key]; !ok {
			return current, fmt.Errorf("%w: unknown setting %s", ErrInvalid, key)
		}
		fields[key] = value
	}
	data, err = json.Marshal(fields)
	if err != nil {
		return current, err
	}
	if err = json.Unmarshal(data, &current); err != nil {
		return current, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err = current.Validate(current.AutoPreview); err != nil {
		return current, err
	}
	_, err = s.DB.Exec(`INSERT INTO environment_settings(project,settings,enabledAt) VALUES(?,?,unixepoch()) ON CONFLICT(project) DO UPDATE SET settings=excluded.settings,enabledAt=CASE WHEN json_extract(excluded.settings,'$.autoPreview')=1 AND COALESCE(json_extract(environment_settings.settings,'$.autoPreview'),0)=0 THEN unixepoch() ELSE enabledAt END`, project, string(data))
	return current, err
}

const columns = `id,COALESCE(project,0),COALESCE(task,0),COALESCE(job,0),requestKey,name,repo,branch,sourceCommit,includeChanges,digest,settings,state,desired,image,testImage,testState,testExitCode,url,error,pinned,expiresAt,createdAt,updatedAt`

func scan(scanner interface{ Scan(...any) error }) (*Environment, error) {
	e := &Environment{}
	var settings string
	err := scanner.Scan(&e.ID, &e.ProjectID, &e.TaskID, &e.JobID, &e.RequestKey, &e.Name, &e.Repo, &e.Branch, &e.Commit, &e.IncludeChanges, &e.Digest, &settings, &e.State, &e.Desired, &e.Image, &e.TestImage, &e.TestState, &e.TestExitCode, &e.URL, &e.Error, &e.Pinned, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
	if err == nil {
		err = json.Unmarshal([]byte(settings), &e.Settings)
	}
	return e, err
}
func (s *Store) Get(id int) (*Environment, error) {
	return scan(s.DB.QueryRow(`SELECT `+columns+` FROM task_environment WHERE id=?`, id))
}
func (s *Store) Existing(project int, key string) (*Environment, error) {
	return scan(s.DB.QueryRow(`SELECT `+columns+` FROM task_environment WHERE project=? AND requestKey=?`, project, key))
}
func (s *Store) List(project, task int, active bool) ([]Environment, error) {
	query := `SELECT ` + columns + ` FROM task_environment WHERE state<>'deleted'`
	args := []any{}
	if project > 0 {
		query += ` AND project=?`
		args = append(args, project)
	}
	if task > 0 {
		query += ` AND task=?`
		args = append(args, task)
	}
	if active {
		query += ` AND NOT(state='stopped' AND desired='stopped')`
	}
	query += ` ORDER BY id DESC`
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Environment{}
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *e)
	}
	return result, rows.Err()
}
func nullable(id int) any {
	if id > 0 {
		return id
	}
	return nil
}
func (s *Store) Insert(e *Environment) (*Environment, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if previous, err := scan(tx.QueryRow(`SELECT `+columns+` FROM task_environment WHERE project=? AND requestKey=?`, e.ProjectID, e.RequestKey)); err == nil {
		return previous, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	var count int
	// Keep the reservation until cleanup confirms there are no running pods.
	// Failed or stopping operations can still have workloads in Kubernetes.
	if err = tx.QueryRow(`SELECT COUNT(*) FROM task_environment WHERE project=? AND state NOT IN ('stopped','deleted')`, e.ProjectID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= e.Settings.MaxRunning {
		return nil, fmt.Errorf("%w: stop an environment first; this project allows %d running previews", ErrConflict, e.Settings.MaxRunning)
	}
	settings, _ := json.Marshal(e.Settings)
	created, err := scan(tx.QueryRow(`INSERT INTO task_environment(project,task,job,requestKey,name,repo,branch,sourceCommit,includeChanges,settings,expiresAt) VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING `+columns, e.ProjectID, nullable(e.TaskID), nullable(e.JobID), e.RequestKey, e.Name, e.Repo, e.Branch, e.Commit, e.IncludeChanges, string(settings), time.Now().Add(time.Duration(e.Settings.RetentionHours)*time.Hour).Unix()))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// Observations cannot undo a stop/delete requested during an external operation.
func (s *Store) Observe(e *Environment) error {
	_, err := s.DB.Exec(`UPDATE task_environment SET state=?,digest=?,image=?,testImage=?,testState=?,testExitCode=?,url=?,error=?,updatedAt=unixepoch() WHERE id=? AND desired=?`, e.State, e.Digest, e.Image, e.TestImage, e.TestState, e.TestExitCode, e.URL, e.Error, e.ID, e.Desired)
	return err
}
func (s *Store) Log(id int, message string) {
	_, _ = s.DB.Exec(`UPDATE task_environment SET logs=substr(logs || ?, -262144) WHERE id=?`, message, id)
}
func (s *Store) Logs(id int) (string, error) {
	var logs string
	err := s.DB.QueryRow(`SELECT logs FROM task_environment WHERE id=?`, id).Scan(&logs)
	return logs, err
}

func (s *Store) Action(id int, action string) error {
	e, err := s.Get(id)
	if err != nil {
		return err
	}
	if e.Desired == "deleted" || e.State == "deleted" {
		if action == "delete" {
			return nil
		}
		return fmt.Errorf("%w: environment is being deleted", ErrConflict)
	}
	switch action {
	case "stop":
		_, err = s.DB.Exec(`UPDATE task_environment SET desired='stopped',state='stopping',url='',error='',updatedAt=unixepoch() WHERE id=? AND desired<>'deleted'`, id)
	case "delete":
		_, err = s.DB.Exec(`UPDATE task_environment SET desired='deleted',state='deleting',url='',error='',updatedAt=unixepoch() WHERE id=?`, id)
	case "start":
		if e.TestState == "failed" {
			return fmt.Errorf("%w: create a fresh preview to rerun failed tests against updated source", ErrConflict)
		}
		if e.State != "stopped" && e.State != "failed" {
			return fmt.Errorf("%w: stop the environment before restarting", ErrConflict)
		}
		result, err := s.DB.Exec(`UPDATE task_environment SET desired='running',state='queued',error='',expiresAt=?,updatedAt=unixepoch() WHERE id=? AND state IN ('stopped','failed') AND desired<>'deleted' AND (SELECT COUNT(*) FROM task_environment WHERE project=? AND id<>? AND state NOT IN ('stopped','deleted'))<?`, time.Now().Add(time.Duration(e.Settings.RetentionHours)*time.Hour).Unix(), id, e.ProjectID, id, e.Settings.MaxRunning)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return fmt.Errorf("%w: stop another environment before starting this one", ErrConflict)
		}
	case "touch":
		_, err = s.DB.Exec(`UPDATE task_environment SET expiresAt=?,updatedAt=unixepoch() WHERE id=?`, time.Now().Add(time.Duration(e.Settings.RetentionHours)*time.Hour).Unix(), id)
	default:
		return fmt.Errorf("%w: unknown action", ErrInvalid)
	}
	return err
}
func (s *Store) Edit(id int, name *string, pinned *bool) error {
	if name != nil && (len(strings.TrimSpace(*name)) == 0 || len(*name) > 120) {
		return fmt.Errorf("%w: name must be 1–120 characters", ErrInvalid)
	}
	result, err := s.DB.Exec(`UPDATE task_environment SET name=COALESCE(?,name),pinned=COALESCE(?,pinned),updatedAt=unixepoch() WHERE id=? AND state<>'deleted'`, name, pinned, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Store) Expire(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE task_environment SET desired='stopped',state='stopping',url='',updatedAt=unixepoch() WHERE desired='running' AND pinned=0 AND expiresAt<unixepoch()`)
	return err
}

func (s *Store) Bulk(project int, ids []int, action string) (models.BulkResult, error) {
	result := models.BulkResult{}
	if action != "stop" && action != "delete" {
		return result, fmt.Errorf("%w: unsupported bulk action", ErrInvalid)
	}
	if len(ids) == 0 || len(ids) > 100 {
		return result, fmt.Errorf("%w: select 1–100 environments", ErrInvalid)
	}
	unique := map[int]bool{}
	args := []any{}
	for _, id := range ids {
		if id <= 0 {
			return result, ErrInvalid
		}
		if !unique[id] {
			unique[id] = true
			args = append(args, id)
		}
	}
	desired, state := "stopped", "stopping"
	if action == "delete" {
		desired, state = "deleted", "deleting"
	}
	params := append([]any{desired, state, project}, args...)
	res, err := s.DB.Exec(`UPDATE task_environment SET desired=?,state=?,url='',error='',updatedAt=unixepoch() WHERE project=? AND id IN (`+strings.TrimRight(strings.Repeat("?,", len(args)), ",")+`) AND desired<>'deleted' AND state<>'deleted'`, params...)
	if err != nil {
		return result, err
	}
	n, err := res.RowsAffected()
	result.Success = int(n)
	result.Skipped = len(unique) - int(n)
	return result, err
}
