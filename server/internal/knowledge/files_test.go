package knowledge

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"testing"
)

func TestFilesPreserveOriginalAndCascade(t *testing.T) {
	s := testStore(t)
	raw := []byte("Fabiana wants us to add the export button.\nKeep the original request.")
	d, err := s.UploadDocument(101, "request.txt", raw)
	if err != nil || d.File == nil || d.File.Size != len(raw) {
		t.Fatalf("%+v %v", d, err)
	}
	if !bytes.Contains(d.Body, []byte("Fabiana")) {
		t.Fatal("text was not imported")
	}
	title := "Fabiana’s request"
	d, err = s.UpdateDocument(101, d.ID, DocumentPatch{Title: &title, Revision: d.Revision})
	if err != nil {
		t.Fatal(err)
	}
	file, content, err := s.DocumentContent(101, d.ID)
	if err != nil || file.Name != "request.txt" || !bytes.Equal(content, raw) {
		t.Fatalf("%+v %s %v", file, content, err)
	}
	if _, _, err = s.DocumentContent(102, d.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	s.ProjectScope = 102
	if _, _, err = s.DocumentContent(101, d.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err = s.Documents(101); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	s.ProjectScope = 0
	list, err := s.Documents(101)
	if err != nil || len(list) != 1 || list[0].File == nil {
		t.Fatalf("%+v %v", list, err)
	}
	if err = s.DeleteDocument(101, d.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.DeleteDocument(101, d.ID, d.Revision); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM project_document_file").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan: %d %v", count, err)
	}
}
func TestUploadFormatsAndLimits(t *testing.T) {
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for _, name := range []string{"[Content_Types].xml", "word/document.xml"} {
		w, _ := z.Create(name)
		w.Write([]byte("<root/>"))
	}
	z.Close()
	valid := []struct {
		name string
		data []byte
	}{
		{"a.pdf", []byte("%PDF-1.7\n%%EOF")},
		{"a.doc", []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}},
		{"a.docx", archive.Bytes()},
		{"a.png", []byte("\x89PNG\r\n\x1a\n0123456789")},
		{"a.md", []byte("# Request\nFabiana wants this")},
	}
	for _, tc := range valid {
		if _, err := inspectFile(tc.name, tc.data); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"a.pdf", []byte("<script>alert(1)</script>")}, {"a.docx", []byte("PK fake zip")}, {"a.svg", []byte("<svg/>")}, {"a.txt", []byte{0xff}}, {"a.md", []byte{0}}, {"a.png", nil}, {"a.pdf", bytes.Repeat([]byte{'x'}, MaxFileBytes+1)},
	} {
		if _, err := inspectFile(tc.name, tc.data); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted %s: %v", tc.name, err)
		}
	}
	f, err := inspectFile("../../a.pdf", valid[0].data)
	if err != nil || f.Name != "a.pdf" {
		t.Fatalf("unsafe name: %+v %v", f, err)
	}
}
