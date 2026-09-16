package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/waseem-polus/aycorn/server/internal/jobs"
)

func (app *app) jobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/project/{projectId}/automation/templates", app.templateList)
	mux.HandleFunc("POST /api/project/{projectId}/automation/templates", app.templateList)
	mux.HandleFunc("GET /api/project/{projectId}/automation/templates/{id}", app.templateDetail)
	mux.HandleFunc("PUT /api/project/{projectId}/automation/templates/{id}", app.templateDetail)
	mux.HandleFunc("DELETE /api/project/{projectId}/automation/templates/{id}", app.templateDetail)
	mux.HandleFunc("POST /api/project/{projectId}/automation/templates/{id}/instantiate", app.instantiateTemplate)
	mux.HandleFunc("POST /api/project/{projectId}/automation/templates/{id}/job", app.templateJob)
	mux.HandleFunc("GET /api/project/{projectId}/automation/jobs", app.scheduledJobList)
	mux.HandleFunc("GET /api/project/{projectId}/automation/jobs/{id}", app.scheduledJobDetail)
	mux.HandleFunc("PUT /api/project/{projectId}/automation/jobs/{id}", app.scheduledJobDetail)
	mux.HandleFunc("DELETE /api/project/{projectId}/automation/jobs/{id}", app.scheduledJobDetail)
	mux.HandleFunc("GET /api/project/{projectId}/automation/jobs/{id}/runs", app.scheduledJobRuns)
	mux.HandleFunc("POST /api/project/{projectId}/automation/jobs/{id}/run", app.fireJob)
}

func jobRouteIDs(w http.ResponseWriter, r *http.Request) (project, id int, ok bool) {
	project, err := strconv.Atoi(r.PathValue("projectId"))
	if err != nil || project <= 0 {
		http.Error(w, "invalid project", 400)
		return
	}
	if raw := r.PathValue("id"); raw != "" {
		id, err = strconv.Atoi(raw)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", 400)
			return
		}
	}
	return project, id, true
}
func decodeJobInput(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 400000))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "invalid request", 400)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "expected one request", 400)
		return false
	}
	return true
}
func (app *app) templateList(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		t, err := app.jobService.CreateTemplate(project)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 201, t)
		return
	}
	list, err := app.jobService.Templates(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}
func (app *app) templateDetail(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := app.jobService.DeleteTemplate(project, id); err != nil {
			respondErr(w, err)
			return
		}
		w.WriteHeader(204)
	case http.MethodPut:
		var t jobs.Template
		if !decodeJobInput(w, r, &t) {
			return
		}
		saved, err := app.jobService.UpdateTemplate(project, id, t)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, saved)
	default:
		t, err := app.jobService.Template(project, id)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, t)
	}
}
func (app *app) instantiateTemplate(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	task, err := app.jobService.Instantiate(r.Context(), project, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]int{"taskId": task})
}
func (app *app) templateJob(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	j, err := app.jobService.CreateJob(project, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 201, j)
}
func (app *app) scheduledJobList(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	list, err := app.jobService.Jobs(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}
func (app *app) scheduledJobDetail(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := app.jobService.DeleteJob(project, id); err != nil {
			respondErr(w, err)
			return
		}
		w.WriteHeader(204)
	case http.MethodPut:
		var j jobs.Job
		if !decodeJobInput(w, r, &j) {
			return
		}
		saved, err := app.jobService.UpdateJob(r.Context(), project, id, j)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, saved)
	default:
		j, err := app.jobService.Job(project, id)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, j)
	}
}
func (app *app) scheduledJobRuns(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	list, err := app.jobService.Runs(project, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}
func (app *app) fireJob(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	var input struct {
		Key string `json:"key"`
	}
	if !decodeJobInput(w, r, &input) {
		return
	}
	task, err := app.jobService.Fire(r.Context(), project, id, "manual", input.Key)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 202, map[string]int{"taskId": task})
}
