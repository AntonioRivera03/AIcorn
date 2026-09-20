package knowledge

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type TaskLink struct {
	ID        int    `json:"id"`
	TaskID    int    `json:"taskId"`
	URL       string `json:"url"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Reference string `json:"reference"`
	Revision  int    `json:"revision"`
	CreatedAt string `json:"createdAt"`
}
type LinkInput struct {
	URL      string `json:"url"`
	Label    string `json:"label"`
	Revision int    `json:"revision,omitempty"`
}

var githubPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// NormalizeGitHubURL accepts navigable GitHub PR/branch references only. It never
// follows the URL or fetches arbitrary network locations.
func NormalizeGitHubURL(raw string) (canonical, kind, reference string, err error) {
	invalid := func() (string, string, string, error) {
		return "", "", "", fmt.Errorf("%w: use a GitHub pull request or branch URL", ErrInvalid)
	}
	if len(raw) > 2048 {
		return invalid()
	}
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || (strings.ToLower(u.Host) != "github.com" && strings.ToLower(u.Host) != "www.github.com") {
		return invalid()
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	if len(parts) < 4 || !githubPart.MatchString(parts[0]) || !githubPart.MatchString(parts[1]) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return invalid()
	}
	parts[0], parts[1] = strings.ToLower(parts[0]), strings.ToLower(parts[1])
	switch parts[2] {
	case "pull":
		if len(parts) != 4 {
			return invalid()
		}
		n, e := strconv.ParseUint(parts[3], 10, 63)
		if e != nil || n == 0 {
			return invalid()
		}
		parts[3] = strconv.FormatUint(n, 10)
		kind = "pull_request"
		reference = parts[0] + "/" + parts[1] + " #" + parts[3]
	case "tree":
		for _, part := range parts[3:] {
			if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `\~^:?*[`) || strings.IndexFunc(part, unicode.IsControl) >= 0 || strings.Contains(part, " ") {
				return invalid()
			}
		}
		kind = "branch"
		reference = parts[0] + "/" + parts[1] + " · " + strings.Join(parts[3:], "/")
	default:
		return invalid()
	}
	u = &url.URL{Scheme: "https", Host: "github.com", Path: "/" + strings.Join(parts, "/")}
	return u.String(), kind, reference, nil
}

const linkColumns = "id,task,url,kind,label,revision,createdAt"

func scanLink(row interface{ Scan(...any) error }) (TaskLink, error) {
	var link TaskLink
	err := row.Scan(&link.ID, &link.TaskID, &link.URL, &link.Kind, &link.Label, &link.Revision, &link.CreatedAt)
	if err == nil {
		_, _, link.Reference, _ = NormalizeGitHubURL(link.URL)
	}
	return link, err
}
func (s *Store) Links(task int) ([]TaskLink, error) {
	var exists int
	if err := s.DB.QueryRow("SELECT id FROM task WHERE id=?", task).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT "+linkColumns+" FROM task_github_link WHERE task=? ORDER BY id", task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []TaskLink{}
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// actorJob=nil denotes a human API edit. MCP supplies a job ID (or zero for an
// external agent); ownership is checked atomically with each mutation.
func (s *Store) PutLink(task, id int, in LinkInput, actorJob *int) (TaskLink, error) {
	canonical, kind, _, err := NormalizeGitHubURL(in.URL)
	if err != nil {
		return TaskLink{}, err
	}
	if len(in.Label) > 300 || strings.ContainsRune(in.Label, 0) {
		return TaskLink{}, fmt.Errorf("%w: link label is too long", ErrInvalid)
	}
	tx, err := taskownership.Begin(s.DB)
	if err != nil {
		return TaskLink{}, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRow("SELECT t.id FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND (?=0 OR c.project=?)", task, s.ProjectScope, s.ProjectScope).Scan(&exists); err != nil {
		return TaskLink{}, err
	}
	if actorJob != nil {
		if err = taskownership.Check(tx, task, *actorJob, s.ChatTurnID); err != nil {
			return TaskLink{}, err
		}
	}
	var link TaskLink
	if id == 0 {
		_, err = tx.Exec("INSERT INTO task_github_link(task,url,kind,label) VALUES(?,?,?,?) ON CONFLICT(task,url) DO NOTHING", task, canonical, kind, strings.TrimSpace(in.Label))
		if err == nil {
			link, err = scanLink(tx.QueryRow("SELECT "+linkColumns+" FROM task_github_link WHERE task=? AND url=?", task, canonical))
		}
	} else {
		if in.Revision <= 0 {
			return link, fmt.Errorf("%w: provide the link revision", ErrInvalid)
		}
		link, err = scanLink(tx.QueryRow("UPDATE task_github_link SET url=?,kind=?,label=?,revision=revision+1 WHERE task=? AND id=? AND revision=? RETURNING "+linkColumns, canonical, kind, strings.TrimSpace(in.Label), task, id, in.Revision))
		if errors.Is(err, sql.ErrNoRows) {
			if lookup := tx.QueryRow("SELECT id FROM task_github_link WHERE task=? AND id=?", task, id).Scan(&exists); lookup != nil {
				return link, lookup
			}
			return link, ErrConflict
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return link, fmt.Errorf("%w: this task already has that link", ErrInvalid)
		}
		return link, err
	}
	return link, tx.Commit()
}
func (s *Store) DeleteLink(task, id, revision int, actorJob *int) error {
	tx, err := taskownership.Begin(s.DB)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if actorJob != nil {
		if err = taskownership.Check(tx, task, *actorJob, s.ChatTurnID); err != nil {
			return err
		}
	}
	var exists int
	if err = tx.QueryRow("SELECT l.id FROM task_github_link l JOIN task t ON t.id=l.task JOIN checklist c ON c.id=t.checklist WHERE l.task=? AND l.id=? AND (?=0 OR c.project=?)", task, id, s.ProjectScope, s.ProjectScope).Scan(&exists); err != nil {
		return err
	}
	res, err := tx.Exec("DELETE FROM task_github_link WHERE task=? AND id=? AND revision=?", task, id, revision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
