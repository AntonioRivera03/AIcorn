package main

import (
	"net/http"

	"github.com/waseem-polus/aycorn/server/internal/mcptools"
)

func (app *app) getMCPTools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, mcptools.All())
}
