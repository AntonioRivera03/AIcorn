package main

import (
	"bytes"
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentUploadDownloadScopeAndRange(t *testing.T) {
	a := aiTestApp(t, "")
	a.projectRepo = a.aiService.Projects
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "brief.pdf")
	part.Write([]byte("%PDF-1.7\noriginal-content\n%%EOF"))
	form.Close()
	req := httptest.NewRequest("POST", "/api/documents/project/1/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var d knowledge.Document
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	url := "/api/documents/project/1/" + fmtInt(d.ID) + "/file"
	for _, tc := range []struct {
		url, rangeHeader string
		status           int
	}{{url, "", 200}, {url, "bytes=0-4", 206}, {strings.Replace(url, "project/1/", "project/999/", 1), "", 404}} {
		req = httptest.NewRequest("GET", tc.url, nil)
		req.Header.Set("Range", tc.rangeHeader)
		w = httptest.NewRecorder()
		a.routes().ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %d %s", tc.url, w.Code, w.Body.String())
		}
		if tc.status < 300 && (!strings.Contains(w.Header().Get("Content-Disposition"), "inline") || w.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatal(w.Header())
		}
		if tc.status == 206 && w.Body.String() != "%PDF-" {
			t.Fatal(w.Body.String())
		}
	}
	req = httptest.NewRequest("GET", url+"?download=1", nil)
	w = httptest.NewRecorder()
	a.routes().ServeHTTP(w, req)
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w.Header())
	}
	req = httptest.NewRequest("POST", "/api/documents/project/1/upload", strings.NewReader("not multipart"))
	w = httptest.NewRecorder()
	a.routes().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
