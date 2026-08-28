package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

// withCommon centralizes the cross-cutting handler preamble that was previously
// copy-pasted into every handler: permissive CORS for the localhost SPA and
// request logging. Applied once around the whole mux in routes().
func withCommon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		log.Println(r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
	})
}

// writeJSON marshals v and writes it with the given status. It is the single
// place that owns the "marshal failed -> 500" tail that was duplicated in
// every handler.
func writeJSON(w http.ResponseWriter, status int, v any) {
	res, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(res)
}

func httpStatusForError(err error) int {
	switch {
	case errors.Is(err, sql.ErrNoRows),
		errors.Is(err, services.ErrInvalidTask),
		errors.Is(err, services.ErrInvalidPersona),
		errors.Is(err, services.ErrJobNotFound):
		return http.StatusNotFound
	case errors.Is(err, services.ErrInvalidStageType),
		errors.Is(err, services.ErrInvalidPersonaHarness),
		errors.Is(err, services.ErrInvalidPersonaModel),
		errors.Is(err, services.ErrInvalidPersonaAgent),
		errors.Is(err, services.ErrInvalidStageColor),
		errors.Is(err, services.ErrCannotDeleteOpenStage),
		errors.Is(err, services.ErrInvalidMoveDestination),
		errors.Is(err, services.ErrInvalidStageOrder),
		errors.Is(err, services.ErrNoOpenStage),
		errors.Is(err, services.ErrInvalidStageMapping),
		errors.Is(err, services.ErrTransferTypeRequired),
		errors.Is(err, services.ErrInvalidTransferType),
		errors.Is(err, services.ErrInvalidProjectView),
		errors.Is(err, services.ErrRepoPathMissing),
		errors.Is(err, services.ErrRepoInvalid):
		return http.StatusBadRequest
	case errors.Is(err, services.ErrStageHasTasks):
		return http.StatusUnprocessableEntity
	case errors.Is(err, services.ErrWorkflowInUse),
		errors.Is(err, services.ErrDuplicateRelationship),
		errors.Is(err, services.ErrStageConflict),
		errors.Is(err, services.ErrInvalidJobStatus),
		errors.Is(err, services.ErrJobStatusConflict),
		errors.Is(err, services.ErrNoPersonaBound),
		errors.Is(err, services.ErrJobAlreadyPending):
		return http.StatusConflict
	case errors.Is(err, services.ErrDefaultTaskType):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func respondErr(w http.ResponseWriter, err error) {
	log.Println(err.Error())
	msg := err.Error()
	if httpStatusForError(err) == http.StatusInternalServerError && msg == "sql: no rows in result set" {
		msg = "not found"
	}
	if httpStatusForError(err) == http.StatusNotFound && msg == "sql: no rows in result set" {
		msg = "not found"
	}
	http.Error(w, msg, httpStatusForError(err))
}
