package main

import (
	"context"
	"net/http"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/repolink"
)

// A project's repository link: Personal (a local checkout) or Official (a
// GitHub repository Aycorn clones). See Documentation/repo-linking.md.
func (app *app) repositoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/project/{projectId}/settings/repository", app.getRepository)
	mux.HandleFunc("PUT /api/project/{projectId}/settings/repository", app.putRepository)
	mux.HandleFunc("POST /api/project/{projectId}/settings/repository/fetch", app.fetchRepository)
}

func (app *app) getRepository(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	status, err := app.repositoryService.Status(r.Context(), project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// putRepository changes the link. Changing an Official link deletes Aycorn's
// old clone, so the app confirms first.
func (app *app) putRepository(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	var patch repolink.Patch
	if !decodeJSONInput(w, r, &patch) {
		return
	}
	status, err := app.repositoryService.Update(r.Context(), project, patch)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// fetchRepository clones or fetches an Official link now. A long clone keeps
// going after the response, which then reports it as still syncing.
func (app *app) fetchRepository(w http.ResponseWriter, r *http.Request) {
	project, err := positiveID(r, "projectId")
	if err != nil {
		respondErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	status, err := app.repositoryService.Fetch(ctx, project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
