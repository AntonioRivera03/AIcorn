package main

import (
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"net/http"
	"strconv"
)

func (app *app) getAISettings(w http.ResponseWriter, r *http.Request) {
	settings, err := app.aiService.Jobs.AISettings()
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings, "engine": app.aiService.Health(r.Context(), settings.Executable)})
}
func (app *app) putAISettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var settings models.AISettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64000)).Decode(&settings); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := app.aiService.UpdateSettings(settings); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
func (app *app) startAIRun(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.Atoi(r.PathValue("taskId"))
	if err != nil {
		http.Error(w, "invalid task", 400)
		return
	}
	defer r.Body.Close()
	var input services.AIRunInput
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64000)).Decode(&input); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	job, err := app.aiService.Start(r.Context(), taskID, input)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}
func (app *app) cancelAIRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("jobId"))
	if err != nil {
		http.Error(w, "invalid run", 400)
		return
	}
	if _, err := app.aiService.Jobs.FindOne(id); err != nil {
		respondErr(w, err)
		return
	}
	ok, err := app.aiService.Jobs.CancelAI(id)
	if err != nil {
		respondErr(w, err)
		return
	}
	if !ok {
		http.Error(w, "run is already finished or stopping", 409)
		return
	}
	writeJSON(w, http.StatusOK, true)
}
