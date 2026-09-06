package main

import (
	"net/http"
	"strconv"
	"strings"

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

	projectIDStr := q.Get("projectId")
	if projectIDStr == "" {
		projectIDStr = q.Get("projectID")
	}
	if projectIDStr != "" {
		projectID, err := strconv.Atoi(projectIDStr)
		if err != nil {
			http.Error(w, "invalid projectId", http.StatusBadRequest)
			return
		}
		statusParam := q.Get("status")
		var jobs []models.AgentJob
		if q.Get("latest") == "1" {
			jobs, err = app.agentJobService.LatestByProject(projectID)
		} else if statusParam == "" {
			jobs, err = app.agentJobService.ListActiveByProject(projectID)
		} else {
			parts := strings.Split(statusParam, ",")
			for i, p := range parts {
				parts[i] = strings.TrimSpace(p)
			}
			jobs, err = app.agentJobService.ListFiltered(&projectID, parts)
		}
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
		trimmed := strings.TrimSpace(status)
		if strings.Contains(trimmed, ",") || trimmed == "active" {
			parts := strings.Split(trimmed, ",")
			for i, p := range parts {
				parts[i] = strings.TrimSpace(p)
			}
			jobs, err := app.agentJobService.ListFiltered(nil, parts)
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
