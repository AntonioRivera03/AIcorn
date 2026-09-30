package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

func TestSwitchingHarnessResetsAgentModelsAndBlocksClaudeCodeRuns(t *testing.T) {
	app := aiTestApp(t, "")
	db := app.aiService.Jobs.DB
	if _, err := db.Exec("UPDATE persona SET model='gpt-5.5' WHERE builtin_role='coder'"); err != nil {
		t.Fatal(err)
	}
	put := func(body string) int {
		rec := httptest.NewRecorder()
		app.routes().ServeHTTP(rec, httptest.NewRequest("PUT", "/api/ai/settings", strings.NewReader(body)))
		return rec.Code
	}
	if code := put(`{"harness":"claude-code","model":"gpt-5.5","timeoutSeconds":300}`); code != 400 {
		t.Fatal("accepted an OpenAI model for Claude Code", code)
	}
	if code := put(`{"harness":"claude-code","model":"claude-sonnet-5","timeoutSeconds":300}`); code != 200 {
		t.Fatal("rejected Claude Code settings", code)
	}
	var coderModel string
	if err := db.QueryRow("SELECT model FROM persona WHERE builtin_role='coder'").Scan(&coderModel); err != nil || coderModel != "" {
		t.Fatalf("switching harness kept the Codex model %q: %v", coderModel, err)
	}
	settings, err := app.aiService.Settings()
	if err != nil || settings.Harness != models.PersonaHarnessClaudeCode || settings.Model != "claude-sonnet-5" {
		t.Fatalf("settings not saved: %+v %v", settings, err)
	}
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/ai/tasks/1/runs", strings.NewReader(`{"intent":"ask","instruction":"Explain"}`)))
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "Claude Code") {
		t.Fatalf("Claude Code run was not blocked: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHarnessModelsFallBackToFixedLists(t *testing.T) {
	app := aiTestApp(t, "")
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/ai/harnesses/claude-code/models", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"claude-opus-5-5"`) || !strings.Contains(rec.Body.String(), `"live":false`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/ai/harnesses/opencode/models", nil))
	if rec.Code != 404 {
		t.Fatal("listed models for an unknown harness", rec.Code)
	}
}
