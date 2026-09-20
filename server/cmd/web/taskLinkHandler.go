package main

import (
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"net/http"
	"strconv"
)

func (app *app) taskLinkRoutes(mux *http.ServeMux) {
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /api/task-links/task/{taskId}", app.taskLinks)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		mux.HandleFunc(method+" /api/task-links/task/{taskId}/{id}", app.taskLinks)
	}
}
func (app *app) taskLinks(w http.ResponseWriter, r *http.Request) {
	task, err := strconv.Atoi(r.PathValue("taskId"))
	if err != nil || task <= 0 {
		http.Error(w, "invalid task", 400)
		return
	}
	id := 0
	if raw := r.PathValue("id"); raw != "" {
		id, err = strconv.Atoi(raw)
		if err != nil || id <= 0 {
			http.Error(w, "invalid link", 400)
			return
		}
	}
	s := knowledge.Store{DB: app.taskService.TaskRepo.DB}
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var in knowledge.LinkInput
		if !decodeJSONInput(w, r, &in) {
			return
		}
		link, err := s.PutLink(task, id, in, nil)
		if err != nil {
			respondErr(w, err)
			return
		}
		status := 200
		if r.Method == http.MethodPost {
			status = 201
		}
		writeJSON(w, status, link)
	case http.MethodDelete:
		revision, err := strconv.Atoi(r.Header.Get("If-Match"))
		if err != nil || revision <= 0 {
			http.Error(w, "provide the link revision in If-Match", 400)
			return
		}
		if err = s.DeleteLink(task, id, revision, nil); err != nil {
			respondErr(w, err)
			return
		}
		w.WriteHeader(204)
	default:
		links, err := s.Links(task)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, links)
	}
}
