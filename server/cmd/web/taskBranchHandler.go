package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (app *app) getTaskBranches(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.Atoi(r.PathValue("taskId"))
	if err != nil || taskID <= 0 {
		http.Error(w, "invalid task", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	branches, err := app.aiService.TaskBranches(ctx, taskID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, branches)
}
func mergeIDs(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	taskID, err := strconv.Atoi(r.PathValue("taskId"))
	jobID, jobErr := strconv.Atoi(r.PathValue("jobId"))
	if err != nil || jobErr != nil || taskID <= 0 || jobID <= 0 {
		http.Error(w, "invalid task or run", 400)
		return 0, 0, false
	}
	return taskID, jobID, true
}
func (app *app) previewTaskMerge(w http.ResponseWriter, r *http.Request) {
	taskID, jobID, ok := mergeIDs(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	preview, err := app.aiService.PreviewTaskMerge(ctx, taskID, jobID, r.URL.Query().Get("target"))
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
func (app *app) mergeTaskBranch(w http.ResponseWriter, r *http.Request) {
	taskID, jobID, ok := mergeIDs(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	var input struct {
		Target string `json:"target"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil || input.Target == "" || input.Token == "" {
		http.Error(w, "destination and reviewed preview are required", 400)
		return
	}
	// Once confirmed, finish this bounded Git operation even if the UI disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 90*time.Second)
	defer cancel()
	result, err := app.aiService.MergeTaskBranch(ctx, taskID, jobID, input.Target, input.Token)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
