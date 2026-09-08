package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/environments"
)

func positiveID(r *http.Request, key string) (int, error) {
	id, err := strconv.Atoi(r.PathValue(key))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid %s", environments.ErrInvalid, key)
	}
	return id, nil
}
func decodeEnvironment(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "Invalid environment request: "+err.Error(), 400)
		return false
	}
	return true
}
func (app *app) environmentProject(r *http.Request) (int, error) {
	if r.PathValue("projectId") != "" {
		return positiveID(r, "projectId")
	}
	task, err := positiveID(r, "taskId")
	if err != nil {
		return 0, err
	}
	var project int
	err = app.environmentService.Store.DB.QueryRowContext(r.Context(), `SELECT c.project FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=?`, task).Scan(&project)
	return project, err
}
func (app *app) getEnvironmentSettings(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	settings, err := app.environmentService.Store.Settings(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, settings)
}
func (app *app) putEnvironmentSettings(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	var patch map[string]json.RawMessage
	if !decodeEnvironment(w, r, &patch) {
		return
	}
	settings, err := app.environmentService.Store.UpdateSettings(project, patch)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, settings)
}
func (app *app) checkEnvironmentConnection(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	settings, err := app.environmentService.Store.Settings(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	kubernetes, ok := app.environmentService.Runtime.(*environments.Kubernetes)
	if !ok {
		http.Error(w, "Kubernetes runtime is unavailable", 503)
		return
	}
	writeJSON(w, 200, kubernetes.Check(ctx, settings))
}
func (app *app) getEnvironmentBranches(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if r.URL.Query().Get("details") == "true" {
		sources, err := app.environmentService.BranchSources(ctx, project)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 200, sources)
		return
	}
	branches, err := app.environmentService.Branches(ctx, project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, branches)
}
func (app *app) listEnvironments(w http.ResponseWriter, r *http.Request) {
	project, err := app.environmentProject(r)
	if err != nil {
		respondErr(w, err)
		return
	}
	task := 0
	if r.PathValue("taskId") != "" {
		task, _ = positiveID(r, "taskId")
	}
	items, err := app.environmentService.Store.List(project, task, false)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, struct {
		ProjectID    int                        `json:"projectId"`
		Environments []environments.Environment `json:"environments"`
	}{project, items})
}
func (app *app) createEnvironment(w http.ResponseWriter, r *http.Request) {
	project, err := app.environmentProject(r)
	if err != nil {
		respondErr(w, err)
		return
	}
	var input environments.CreateInput
	if !decodeEnvironment(w, r, &input) {
		return
	}
	if r.PathValue("taskId") != "" {
		input.TaskID, _ = positiveID(r, "taskId")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	e, err := app.environmentService.Create(ctx, project, input)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 202, e)
}
func (app *app) getEnvironment(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(r, "id")
	if err != nil {
		respondErr(w, err)
		return
	}
	e, err := app.environmentService.Store.Get(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, e)
}

func (app *app) environmentSourceStatus(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(r, "id")
	if err != nil {
		respondErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, err := app.environmentService.SourceStatus(ctx, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, status)
}
func (app *app) editEnvironment(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(r, "id")
	if err != nil {
		respondErr(w, err)
		return
	}
	var patch struct {
		Name   *string `json:"name"`
		Pinned *bool   `json:"pinned"`
	}
	if !decodeEnvironment(w, r, &patch) {
		return
	}
	if err = app.environmentService.Store.Edit(id, patch.Name, patch.Pinned); err != nil {
		respondErr(w, err)
		return
	}
	app.getEnvironment(w, r)
}
func (app *app) environmentAction(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(r, "id")
	if err != nil {
		respondErr(w, err)
		return
	}
	action := r.PathValue("action")
	if r.Method == http.MethodDelete {
		action = "delete"
	}
	if action == "rebuild" {
		var input environments.CreateInput
		if !decodeEnvironment(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		e, err := app.environmentService.Rebuild(ctx, id, input)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, 202, e)
		return
	}
	if err = app.environmentService.Store.Action(id, action); err != nil {
		respondErr(w, err)
		return
	}
	app.getEnvironment(w, r)
}
func (app *app) environmentLogs(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(r, "id")
	if err != nil {
		respondErr(w, err)
		return
	}
	e, err := app.environmentService.Store.Get(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	logs, err := app.environmentService.Store.Logs(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	if e.State != "deleted" && e.State != "queued" && e.State != "snapshotting" && e.State != "building" {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		live, err := app.environmentService.Runtime.Logs(ctx, e)
		logs += live
		if err != nil {
			logs += "\nRuntime logs unavailable: " + err.Error()
		}
	}
	writeJSON(w, 200, struct {
		Logs string `json:"logs"`
	}{logs})
}
func (app *app) bulkEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	var input struct {
		IDs    []int  `json:"ids"`
		Action string `json:"action"`
	}
	if !decodeEnvironment(w, r, &input) {
		return
	}
	result, err := app.environmentService.Store.Bulk(project, input.IDs, input.Action)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, result)
}
