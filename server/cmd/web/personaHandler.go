package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

func (app *app) getAllPersonas(w http.ResponseWriter, _ *http.Request) {
	personas, err := app.personaService.GetAll()
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, personas)
}

func (app *app) getPersona(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("personaId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	persona, err := app.personaService.Get(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, persona)
}

func (app *app) postPersona(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	persona := models.Persona{}
	if err := json.NewDecoder(r.Body).Decode(&persona); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	created, err := app.personaService.Create(&persona)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *app) putPersona(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	id, err := strconv.Atoi(r.PathValue("personaId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	persona := models.Persona{}
	if err := json.NewDecoder(r.Body).Decode(&persona); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	persona.ID = id
	updated, err := app.personaService.Update(&persona)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *app) deletePersona(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("personaId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	deleted, err := app.personaService.Delete(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (app *app) bulkCreatePersonas(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	personas := []models.Persona{}
	if err := json.NewDecoder(r.Body).Decode(&personas); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := app.personaService.BulkCreate(personas)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *app) bulkUpdatePersonas(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	personas := []models.Persona{}
	if err := json.NewDecoder(r.Body).Decode(&personas); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := app.personaService.BulkUpdate(personas)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (app *app) bulkDeletePersonas(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ids := []int{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := app.personaService.BulkDelete(ids)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
