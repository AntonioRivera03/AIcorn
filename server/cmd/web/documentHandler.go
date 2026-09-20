package main

import (
	"bytes"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
)

func (app *app) documentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/documents/project/{projectId}/upload", app.documentUpload)
	mux.HandleFunc("GET /api/documents/project/{projectId}/{id}/file", app.documentFile)
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /api/documents/project/{projectId}", app.documentList)
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		mux.HandleFunc(method+" /api/documents/project/{projectId}/{id}", app.documentDetail)
	}
}

func (app *app) documentUpload(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, knowledge.MaxFileBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "upload one multipart file", 400)
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		http.Error(w, "upload one file in the file field", 400)
		return
	}
	content, err := io.ReadAll(io.LimitReader(part, knowledge.MaxFileBytes+1))
	if err != nil || len(content) > knowledge.MaxFileBytes {
		http.Error(w, "file exceeds 20 MiB or upload was interrupted", 413)
		return
	}
	name := part.FileName()
	if _, err := reader.NextPart(); err != io.EOF {
		http.Error(w, "upload exactly one file per request", 400)
		return
	}
	store := knowledge.Store{DB: app.projectRepo.DB}
	doc, err := store.UploadDocument(project, name, content)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 201, doc)
}

func (app *app) documentFile(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	store := knowledge.Store{DB: app.projectRepo.DB}
	file, content, err := store.DocumentContent(project, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("download") != "1" && (file.MediaType == "application/pdf" || file.MediaType == "image/png" || file.MediaType == "image/jpeg" || file.MediaType == "image/gif" || file.MediaType == "image/webp") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", file.MediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, file.Name, time.Time{}, bytes.NewReader(content))
}

func (app *app) documentList(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	store := knowledge.Store{DB: app.projectRepo.DB}
	if r.Method == http.MethodPost {
		doc, err := store.CreateDocument(project)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 201, doc)
		return
	}
	docs, err := store.Documents(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, docs)
}

func (app *app) documentDetail(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	store := knowledge.Store{DB: app.projectRepo.DB}
	switch r.Method {
	case http.MethodPut:
		var patch knowledge.DocumentPatch
		if !decodeJSONInput(w, r, &patch) {
			return
		}
		doc, err := store.UpdateDocument(project, id, patch)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, doc)
	case http.MethodDelete:
		revision, err := strconv.Atoi(r.Header.Get("If-Match"))
		if err != nil || revision <= 0 {
			http.Error(w, "provide the document revision in If-Match", 400)
			return
		}
		if err := store.DeleteDocument(project, id, revision); err != nil {
			respondErr(w, err)
			return
		}
		w.WriteHeader(204)
	default:
		doc, err := store.Document(project, id)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, doc)
	}
}
