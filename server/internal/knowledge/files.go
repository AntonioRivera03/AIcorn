package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
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
	case ".eml":
		_, err := parseEmail(content)
		valid = err == nil
		media = "message/rfc822"
	}
	if !valid {
		return DocumentFile{}, fmt.Errorf("%w: upload a valid PDF, DOC, DOCX, PNG, JPEG, GIF, WebP, email (.eml), or UTF-8 TXT/Markdown file (text up to 128 KB)", ErrInvalid)
	}
	return DocumentFile{Name: name, MediaType: media, Size: len(content)}, nil
}

// MarkdownConverter turns Markdown into rich text bodies (internal/markdown).
type MarkdownConverter interface {
	ToBody(ctx context.Context, markdowns []string) ([]string, error)
}

// UploadDocument keeps the original file and fills in what can be read from
// it: text and Markdown become the editable body, an email its body and
// headers. The title is the file name without its extension (the list shows
// the type), or an email's subject.
func (s *Store) UploadDocument(ctx context.Context, project int, name string, content []byte) (Document, error) {
	file, err := inspectFile(name, content)
	if err != nil {
		return Document{}, err
	}
	if s.ProjectScope > 0 && s.ProjectScope != project {
		return Document{}, sql.ErrNoRows
	}
	title := strings.TrimSuffix(file.Name, path.Ext(file.Name))
	if title == "" {
		title = file.Name
	}
	body := []byte(emptyBody)
	details := DocumentDetails{}
	switch ext := strings.ToLower(path.Ext(file.Name)); {
	case ext == ".md":
		body = s.markdownBody(ctx, string(content))
	case ext == ".txt":
		body = textBody(string(content))
	case ext == ".eml":
		email, err := parseEmail(content)
		if err != nil {
			return Document{}, fmt.Errorf("%w: this email couldn't be read", ErrInvalid)
		}
		if email.Subject != "" {
			title = email.Subject
		}
		details.Email = &email.Details
		body = textBody(email.Text)
	}
	rawDetails, _ := json.Marshal(details)
	tx, err := s.DB.Begin()
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback()
	var id int
	err = tx.QueryRow("INSERT INTO project_document(project,title,body,details) SELECT id,?,?,? FROM project WHERE id=? RETURNING id", truncateTitle(title), string(body), string(rawDetails), project).Scan(&id)
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

const (
	emptyBody    = `[{"type":"p","children":[{"text":""}]}]`
	maxBodyBytes = 256000
)

// markdownBody converts Markdown to rich text, falling back to plain lines
// when the converter isn't available.
func (s *Store) markdownBody(ctx context.Context, markdown string) []byte {
	if s.Markdown != nil {
		if bodies, err := s.Markdown.ToBody(ctx, []string{markdown}); err == nil && len(bodies) == 1 && len(bodies[0]) <= maxBodyBytes && validBody([]byte(bodies[0])) {
			return []byte(bodies[0])
		}
	}
	return textBody(markdown)
}

// textBody makes one paragraph per line, so plain text keeps its shape in
// the editor. Text too long for the editor is cut off with a note; the
// original file is always kept.
func textBody(text string) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return []byte(emptyBody)
	}
	paragraph := func(line string) map[string]any {
		return map[string]any{"type": "p", "children": []any{map[string]string{"text": line}}}
	}
	nodes := []any{}
	size := 0
	for _, line := range strings.Split(text, "\n") {
		size += len(line) + 48
		if size > maxBodyBytes-200 {
			nodes = append(nodes, paragraph("… The rest is in the original file."))
			break
		}
		nodes = append(nodes, paragraph(line))
	}
	body, _ := json.Marshal(nodes)
	return body
}

func truncateTitle(title string) string {
	if len(title) <= 500 {
		return title
	}
	cut := 500
	for cut > 0 && !utf8.RuneStart(title[cut]) {
		cut--
	}
	return title[:cut]
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
