package main

import (
	"net/http"
	"strconv"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

func (app *app) getAgentJobsForTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.Atoi(r.PathValue("taskId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := app.agentJobService.ListByTask(taskID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *app) getAgentJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	taskIDStr := q.Get("taskId")
	status := q.Get("status")

	if taskIDStr != "" {
		taskID, err := strconv.Atoi(taskIDStr)
		if err != nil {
			http.Error(w, "invalid taskId", http.StatusBadRequest)
			return
		}
		resp, err := app.agentJobService.ListByTask(taskID)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if status != "" {
		jobs, err := app.agentJobService.ListByStatus(status)
		if err != nil {
			respondErr(w, err)
			return
		}
		if jobs == nil {
			jobs = []models.AgentJob{}
		}
		writeJSON(w, http.StatusOK, jobs)
		return
	}

	jobs, err := app.agentJobService.ListAll()
	if err != nil {
		respondErr(w, err)
		return
	}
	if jobs == nil {
		jobs = []models.AgentJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (app *app) listAgentJobs(w http.ResponseWriter, r *http.Request) {
	app.getAgentJobs(w, r)
}
