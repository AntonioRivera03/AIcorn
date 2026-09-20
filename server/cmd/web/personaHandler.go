package main

import (
	"encoding/json"
	"io"
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

func (app *app) postPersona(w http.ResponseWriter, _ *http.Request) { fixedAgentResponse(w) }

func fixedAgentResponse(w http.ResponseWriter) {
	http.Error(w, "Agent definitions are managed by Aycorn. Only the model can be changed.", http.StatusForbidden)
}

func (app *app) putPersona(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	id, err := strconv.Atoi(r.PathValue("personaId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var in struct{ Model models.PersonaModel }
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		http.Error(w, "Only Model can be updated.", http.StatusBadRequest)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Expected one model update.", http.StatusBadRequest)
		return
	}
	updated, err := app.personaService.UpdateModel(id, in.Model)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *app) deletePersona(w http.ResponseWriter, _ *http.Request)      { fixedAgentResponse(w) }
func (app *app) bulkCreatePersonas(w http.ResponseWriter, _ *http.Request) { fixedAgentResponse(w) }
func (app *app) bulkUpdatePersonas(w http.ResponseWriter, _ *http.Request) { fixedAgentResponse(w) }
func (app *app) bulkDeletePersonas(w http.ResponseWriter, _ *http.Request) { fixedAgentResponse(w) }
