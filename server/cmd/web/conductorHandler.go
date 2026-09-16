package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (app *app) getConductor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("projectId"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid project", 400)
		return
	}
	board, err := app.conductorService.Board(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (app *app) putConductor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("projectId"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid project", 400)
		return
	}
	defer r.Body.Close()
	var patch map[string]json.RawMessage
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 100000)).Decode(&patch); err != nil || len(patch) == 0 {
		http.Error(w, "invalid settings", 400)
		return
	}
	settings, err := app.conductorService.UpdateSettings(r.Context(), id, patch)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (app *app) bulkConductor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("projectId"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid project", 400)
		return
	}
	defer r.Body.Close()
	var input struct {
		IDs    []int  `json:"ids"`
		Action string `json:"action"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000))
	d.DisallowUnknownFields()
	if err = d.Decode(&input); err != nil {
		http.Error(w, "invalid task selection", 400)
		return
	}
	result, err := app.conductorService.Manage(id, input.IDs, input.Action)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
