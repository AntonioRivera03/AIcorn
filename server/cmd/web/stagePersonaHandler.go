package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (app *app) putStagePersona(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	stageID, err := strconv.Atoi(r.PathValue("stageId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body := struct {
		PersonaID int `json:"personaId"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.PersonaID <= 0 {
		http.Error(w, "personaId must be a positive integer", http.StatusBadRequest)
		return
	}

	if err := app.stageService.BindPersona(stageID, body.PersonaID); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, true)
}

func (app *app) deleteStagePersona(w http.ResponseWriter, r *http.Request) {
	stageID, err := strconv.Atoi(r.PathValue("stageId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	removed, err := app.stageService.UnbindPersona(stageID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removed)
}
