// Package knowledge stores project knowledge independently of board tasks.
package knowledge

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid knowledge item")
var ErrConflict = errors.New("this item changed elsewhere; reload before editing again")

type Store struct {
	DB           *sql.DB
	ProjectScope int // Optional agent scope, enforced again inside link write transactions.
	ChatTurnID   int
	// Markdown converts uploaded .md files to rich text; optional.
	Markdown MarkdownConverter
}

type Document struct {
	ID        int             `json:"id"`
	ProjectID int             `json:"projectId"`
	Title     string          `json:"title"`
	Body      json.RawMessage `json:"body"`
	Tags      []string        `json:"tags"`
	Details   DocumentDetails `json:"details"`
	Revision  int             `json:"revision"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
	File      *DocumentFile   `json:"file,omitempty"`
}

// DocumentDetails are read from an uploaded file when it's imported, and
// shown alongside the document. They aren't editable.
type DocumentDetails struct {
	Email *EmailDetails `json:"email,omitempty"`
}

type DocumentPatch struct {
	Title    *string         `json:"title,omitempty"`
	Body     json.RawMessage `json:"body,omitempty"`
	Tags     *[]string       `json:"tags,omitempty"`
	Revision int             `json:"revision"`
}

const (
	maxTags      = 20
	maxTagLength = 40
)

// normalizeTags trims each tag and drops blanks and case-insensitive
// duplicates, keeping the first spelling.
func normalizeTags(tags []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.Join(strings.Fields(tag), " ")
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxTagLength || strings.ContainsFunc(tag, unicode.IsControl) {
			return nil, fmt.Errorf("%w: tags must be at most %d characters", ErrInvalid, maxTagLength)
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
	}
	if len(out) > maxTags {
		return nil, fmt.Errorf("%w: a document can have at most %d tags", ErrInvalid, maxTags)
	}
	return out, nil
}

const documentColumns = `id,project,title,body,tags,details,revision,timeCreated,timeModified,
COALESCE((SELECT fileName FROM project_document_file WHERE documentId=project_document.id),''),
COALESCE((SELECT mediaType FROM project_document_file WHERE documentId=project_document.id),''),
COALESCE((SELECT byteSize FROM project_document_file WHERE documentId=project_document.id),0)`

func scanDocument(row interface{ Scan(...any) error }) (Document, error) {
	var d Document
	var body, tags, details string
	var file DocumentFile
	err := row.Scan(&d.ID, &d.ProjectID, &d.Title, &body, &tags, &details, &d.Revision, &d.CreatedAt, &d.UpdatedAt, &file.Name, &file.MediaType, &file.Size)
	if err != nil {
		return d, err
	}
	if file.Size > 0 {
		d.File = &file
	}
	d.Body = json.RawMessage(body)
	if json.Unmarshal([]byte(tags), &d.Tags) != nil || d.Tags == nil {
		d.Tags = []string{}
	}
	_ = json.Unmarshal([]byte(details), &d.Details)
	return d, nil
}

func (s *Store) Documents(project int) ([]Document, error) {
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return nil, sql.ErrNoRows
	}
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
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return Document{}, sql.ErrNoRows
	}
	return scanDocument(s.DB.QueryRow("SELECT "+documentColumns+" FROM project_document WHERE project=? AND id=?", project, id))
}

func (s *Store) CreateDocument(project int) (Document, error) {
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return Document{}, sql.ErrNoRows
	}
	return scanDocument(s.DB.QueryRow("INSERT INTO project_document(project) SELECT id FROM project WHERE id=? RETURNING "+documentColumns, project))
}

func (s *Store) UpdateDocument(project, id int, patch DocumentPatch) (Document, error) {
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return Document{}, sql.ErrNoRows
	}
	if patch.Revision <= 0 || (patch.Title == nil && patch.Body == nil && patch.Tags == nil) {
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
	var tags any
	if patch.Tags != nil {
		clean, err := normalizeTags(*patch.Tags)
		if err != nil {
			return Document{}, err
		}
		raw, _ := json.Marshal(clean)
		tags = string(raw)
	}
	d, err := scanDocument(s.DB.QueryRow("UPDATE project_document SET title=COALESCE(?,title),body=COALESCE(?,body),tags=COALESCE(?,tags),revision=revision+1,timeModified=CURRENT_TIMESTAMP WHERE project=? AND id=? AND revision=? RETURNING "+documentColumns, patch.Title, body, tags, project, id, patch.Revision))
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
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return sql.ErrNoRows
	}
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
