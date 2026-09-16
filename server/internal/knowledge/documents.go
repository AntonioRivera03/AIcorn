// Package knowledge stores project knowledge independently of board tasks.
package knowledge

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalid = errors.New("invalid knowledge item")
var ErrConflict = errors.New("this item changed elsewhere; reload before editing again")

type Store struct {
	DB           *sql.DB
	ProjectScope int // Optional agent scope, enforced again inside link write transactions.
}

type Document struct {
	ID        int             `json:"id"`
	ProjectID int             `json:"projectId"`
	Title     string          `json:"title"`
	Body      json.RawMessage `json:"body"`
	Revision  int             `json:"revision"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

type DocumentPatch struct {
	Title    *string         `json:"title,omitempty"`
	Body     json.RawMessage `json:"body,omitempty"`
	Revision int             `json:"revision"`
}

const documentColumns = "id,project,title,body,revision,timeCreated,timeModified"

func scanDocument(row interface{ Scan(...any) error }) (Document, error) {
	var d Document
	var body string
	err := row.Scan(&d.ID, &d.ProjectID, &d.Title, &body, &d.Revision, &d.CreatedAt, &d.UpdatedAt)
	d.Body = json.RawMessage(body)
	return d, err
}

func (s *Store) Documents(project int) ([]Document, error) {
	var exists int
	if err := s.DB.QueryRow("SELECT id FROM project WHERE id=?", project).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT "+documentColumns+" FROM project_document WHERE project=? ORDER BY timeModified DESC,id DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s *Store) Document(project, id int) (Document, error) {
	return scanDocument(s.DB.QueryRow("SELECT "+documentColumns+" FROM project_document WHERE project=? AND id=?", project, id))
}

func (s *Store) CreateDocument(project int) (Document, error) {
	return scanDocument(s.DB.QueryRow("INSERT INTO project_document(project) SELECT id FROM project WHERE id=? RETURNING "+documentColumns, project))
}

func (s *Store) UpdateDocument(project, id int, patch DocumentPatch) (Document, error) {
	if patch.Revision <= 0 || (patch.Title == nil && patch.Body == nil) {
		return Document{}, fmt.Errorf("%w: supply changed fields and a revision", ErrInvalid)
	}
	if patch.Title != nil && (len(*patch.Title) > 500 || strings.ContainsRune(*patch.Title, 0)) {
		return Document{}, fmt.Errorf("%w: title must be at most 500 bytes", ErrInvalid)
	}
	var body any
	if patch.Body != nil {
		if len(patch.Body) > 256000 || !validBody(patch.Body) {
			return Document{}, fmt.Errorf("%w: provide a rich text document up to 256 KB", ErrInvalid)
		}
		body = string(patch.Body)
	}
	d, err := scanDocument(s.DB.QueryRow("UPDATE project_document SET title=COALESCE(?,title),body=COALESCE(?,body),revision=revision+1,timeModified=CURRENT_TIMESTAMP WHERE project=? AND id=? AND revision=? RETURNING "+documentColumns, patch.Title, body, project, id, patch.Revision))
	if errors.Is(err, sql.ErrNoRows) {
		if _, lookup := s.Document(project, id); lookup != nil {
			return d, lookup
		}
		return d, ErrConflict
	}
	return d, err
}

// Reject malformed Slate nodes before they can crash the editor on next load.
func validBody(raw []byte) bool {
	var nodes []map[string]any
	if json.Unmarshal(raw, &nodes) != nil || len(nodes) == 0 {
		return false
	}
	var valid func(map[string]any, int) bool
	valid = func(n map[string]any, depth int) bool {
		if depth > 50 {
			return false
		}
		if _, ok := n["text"].(string); ok {
			return true
		}
		if _, ok := n["type"].(string); !ok {
			return false
		}
		children, ok := n["children"].([]any)
		if !ok || len(children) == 0 {
			return false
		}
		for _, child := range children {
			node, ok := child.(map[string]any)
			if !ok || !valid(node, depth+1) {
				return false
			}
		}
		return true
	}
	for _, node := range nodes {
		if _, ok := node["type"].(string); !ok || !valid(node, 0) {
			return false
		}
	}
	return true
}

func (s *Store) DeleteDocument(project, id, revision int) error {
	res, err := s.DB.Exec("DELETE FROM project_document WHERE project=? AND id=? AND revision=?", project, id, revision)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.Document(project, id); err != nil {
			return err
		}
		return ErrConflict
	}
	return nil
}
