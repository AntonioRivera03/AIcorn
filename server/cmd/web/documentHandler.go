package main

import (
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"net/http"
	"strconv"
)

func (app *app) documentRoutes(mux *http.ServeMux) {
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /api/documents/project/{projectId}", app.documentList)
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		mux.HandleFunc(method+" /api/documents/project/{projectId}/{id}", app.documentDetail)
	}
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
