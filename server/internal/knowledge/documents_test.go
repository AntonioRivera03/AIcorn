package knowledge

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "knowledge.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workflow(id,name) VALUES(101,'Test'); INSERT INTO project(id,name,workflow) VALUES(101,'First',101),(102,'Second',101)"); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db}
}

func TestDocumentsPreserveFieldsAndRejectStaleWrites(t *testing.T) {
	s := testStore(t)
	d, err := s.CreateDocument(101)
	if err != nil || d.Title != "" || d.Revision != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	title := "Architecture decisions"
	d, err = s.UpdateDocument(101, d.ID, DocumentPatch{Title: &title, Revision: d.Revision})
	if err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`[{"type":"p","children":[{"text":"Use project-scoped documents."}]}]`)
	d, err = s.UpdateDocument(101, d.ID, DocumentPatch{Body: body, Revision: d.Revision})
	if err != nil || d.Title != title || string(d.Body) != string(body) {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := s.UpdateDocument(101, d.ID, DocumentPatch{Title: &title, Revision: 1}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.DeleteDocument(101, d.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := s.Document(102, d.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.UpdateDocument(102, d.ID, DocumentPatch{Title: &title, Revision: d.Revision}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := s.DeleteDocument(102, d.ID, d.Revision); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `[null]`, `[{"type":"p"}]`, `[{"text":"leaf"}]`, `[{"type":"p","children":[{"text":4}]}]`} {
		if _, err := s.UpdateDocument(101, d.ID, DocumentPatch{Body: json.RawMessage(raw), Revision: d.Revision}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	items, err := s.Documents(101)
	if err != nil || len(items) != 1 || items[0].Revision != 3 {
		t.Fatalf("%+v %v", items, err)
	}
	if _, err := s.CreateDocument(999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.Documents(999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("DELETE FROM project WHERE id=101"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Document(101, d.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("orphaned document: %v", err)
	}
}
