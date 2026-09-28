package repolink

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SQL for project links and their sync status. It lives with the package,
// like environments.Store, because nothing else reads these columns.

func loadLink(ctx context.Context, db *sql.DB, project int) (Link, error) {
	var link Link
	var mode string
	err := db.QueryRowContext(ctx, `SELECT repoMode, COALESCE(repoPath, ''), repoUrl FROM project WHERE id = ?`, project).Scan(&mode, &link.Path, &link.URL)
	link.Mode = Mode(mode)
	link.Path = strings.TrimSpace(link.Path)
	return link, err
}

func saveLink(ctx context.Context, db *sql.DB, project int, link Link) error {
	res, err := db.ExecContext(ctx, `UPDATE project SET repoMode = ?, repoPath = ?, repoUrl = ? WHERE id = ?`, string(link.Mode), link.Path, link.URL, project)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		err = sql.ErrNoRows
	}
	return err
}

// syncRecord is the outcome of the last clone or fetch of url.
type syncRecord struct {
	url       string
	fetchedAt *time.Time
	err       string
}

func loadSync(ctx context.Context, db *sql.DB, project int) (syncRecord, error) {
	var rec syncRecord
	var fetched sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT url, fetchedAt, error FROM repository_sync WHERE project = ?`, project).Scan(&rec.url, &fetched, &rec.err)
	if errors.Is(err, sql.ErrNoRows) {
		return syncRecord{}, nil
	}
	if fetched.Valid {
		t := time.Unix(fetched.Int64, 0).UTC()
		rec.fetchedAt = &t
	}
	return rec, err
}

// recordSync notes a clone or fetch of url. A success sets the fetch time and
// clears the error; a failure keeps the last successful fetch time of the same
// link, so the status can say how stale the clone is.
func recordSync(ctx context.Context, db *sql.DB, project int, url string, syncErr error) error {
	if syncErr == nil {
		_, err := db.ExecContext(ctx, `INSERT INTO repository_sync(project, url, fetchedAt, error) VALUES(?, ?, unixepoch(), '')
ON CONFLICT(project) DO UPDATE SET url = excluded.url, fetchedAt = excluded.fetchedAt, error = ''`, project, url)
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO repository_sync(project, url, fetchedAt, error) VALUES(?, ?, NULL, ?)
ON CONFLICT(project) DO UPDATE SET fetchedAt = CASE WHEN repository_sync.url = excluded.url THEN repository_sync.fetchedAt END, url = excluded.url, error = excluded.error`, project, url, syncErr.Error())
	return err
}

// agentWorkingIn reports whether an agent run is using the checkout at root.
// Runs record the repository root they work in as requestJson.repoPath.
func agentWorkingIn(ctx context.Context, db *sql.DB, root string) (bool, error) {
	var busy bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_job WHERE status IN ('claimed','running','canceling') AND json_extract(requestJson, '$.repoPath') = ?)`, root).Scan(&busy)
	return busy, err
}

// reposDir is <workspace dir>/repos, beside the workspace's own database like
// its backups and environments. Deriving it from the database itself means a
// workspace can only ever reach its own clones, in any process that opens it.
func reposDir(ctx context.Context, db *sql.DB) (string, error) {
	var file string
	if err := db.QueryRowContext(ctx, `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file); err != nil {
		return "", err
	}
	if file == "" {
		return "", errors.New("official repositories need a workspace database file")
	}
	return filepath.Join(filepath.Dir(file), "repos"), nil
}

func projectDir(repos string, project int) string {
	return filepath.Join(repos, strconv.Itoa(project))
}

func cloneRoot(repos string, project int, repo Repo) string {
	return filepath.Join(projectDir(repos, project), repo.Owner, repo.Name)
}
