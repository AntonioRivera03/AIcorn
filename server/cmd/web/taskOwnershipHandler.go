package main

import (
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"net/http"
)

func (app *app) projectTaskOwners(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	owners, err := taskownership.ListProject(app.taskService.TaskRepo.DB, project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, owners)
}
