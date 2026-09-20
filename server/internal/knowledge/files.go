package knowledge

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxFileBytes = 20 << 20

type DocumentFile struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
}

func inspectFile(name string, content []byte) (DocumentFile, error) {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" || len(name) > 240 || strings.ContainsFunc(name, unicode.IsControl) {
		return DocumentFile{}, fmt.Errorf("%w: invalid file name", ErrInvalid)
	}
	if len(content) == 0 || len(content) > MaxFileBytes {
		return DocumentFile{}, fmt.Errorf("%w: files must be between 1 byte and 20 MiB", ErrInvalid)
	}
	media := http.DetectContentType(content)
	ext := strings.ToLower(path.Ext(name))
	valid := false
	switch ext {
	case ".pdf":
		valid = bytes.HasPrefix(content, []byte("%PDF-"))
		media = "application/pdf"
	case ".doc":
		valid = bytes.HasPrefix(content, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
		media = "application/msword"
	case ".docx":
		archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err == nil {
			parts := map[string]bool{}
			var expanded uint64
			for _, file := range archive.File {
				expanded += file.UncompressedSize64
				if expanded > 100<<20 {
					break
				}
				if file.Name == "[Content_Types].xml" || file.Name == "word/document.xml" {
					// Read the required entries to verify the archive's CRC without extracting it.
					if file.UncompressedSize64 > 10<<20 {
						break
					}
					reader, openErr := file.Open()
					if openErr != nil {
						break
					}
					_, readErr := io.Copy(io.Discard, io.LimitReader(reader, (10<<20)+1))
					reader.Close()
					if readErr != nil {
						break
					}
					parts[file.Name] = true
				}
			}
			valid = expanded <= 100<<20 && parts["[Content_Types].xml"] && parts["word/document.xml"]
		}
		media = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		expected := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
		valid = media == expected[ext]
	case ".txt", ".md":
		valid = utf8.Valid(content) && !bytes.ContainsRune(content, 0) && len(content) <= 128000
		media = "text/plain; charset=utf-8"
	}
	if !valid {
		return DocumentFile{}, fmt.Errorf("%w: upload a valid PDF, DOC, DOCX, PNG, JPEG, GIF, WebP, or UTF-8 TXT/Markdown file (text up to 128 KB)", ErrInvalid)
	}
	return DocumentFile{Name: name, MediaType: media, Size: len(content)}, nil
}

func (s *Store) UploadDocument(project int, name string, content []byte) (Document, error) {
	file, err := inspectFile(name, content)
	if err != nil {
		return Document{}, err
	}
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return Document{}, sql.ErrNoRows
	}
	body := []byte(`[{"type":"p","children":[{"text":""}]}]`)
	if strings.HasPrefix(file.MediaType, "text/plain") {
		body, _ = json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": string(content)}}}})
		if len(body) > 256000 {
			return Document{}, fmt.Errorf("%w: text is too large for the editor", ErrInvalid)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback()
	var id int
	err = tx.QueryRow("INSERT INTO project_document(project,title,body) SELECT id,?,? FROM project WHERE id=? RETURNING id", file.Name, string(body), project).Scan(&id)
	if err != nil {
		return Document{}, err
	}
	if _, err = tx.Exec("INSERT INTO project_document_file(documentId,fileName,mediaType,byteSize,content) VALUES(?,?,?,?,?)", id, file.Name, file.MediaType, file.Size, content); err != nil {
		return Document{}, err
	}
	if err = tx.Commit(); err != nil {
		return Document{}, err
	}
	return s.Document(project, id)
}

func (s *Store) DocumentContent(project, id int) (DocumentFile, []byte, error) {
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return DocumentFile{}, nil, sql.ErrNoRows
	}
	var file DocumentFile
	var content []byte
	err := s.DB.QueryRow("SELECT f.fileName,f.mediaType,f.byteSize,f.content FROM project_document_file f JOIN project_document d ON d.id=f.documentId WHERE d.project=? AND d.id=?", project, id).Scan(&file.Name, &file.MediaType, &file.Size, &content)
	return file, content, err
}
