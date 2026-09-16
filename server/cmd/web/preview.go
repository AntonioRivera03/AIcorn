package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PreviewProtocolVersion = 1 is checked in source before the Aycorn profile builds.
const PreviewProtocolVersion = 1

func previewMode() bool { return os.Getenv("AYCORN_PREVIEW") == "1" }

func (app *app) getPreviewInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, struct {
		Preview  bool   `json:"preview"`
		Branch   string `json:"branch"`
		Revision string `json:"revision"`
		Digest   string `json:"digest"`
		MainURL  string `json:"mainUrl"`
	}{previewMode(), os.Getenv("AYCORN_PREVIEW_BRANCH"), os.Getenv("AYCORN_PREVIEW_REVISION"), os.Getenv("AYCORN_PREVIEW_DIGEST"), os.Getenv("AYCORN_MAIN_URL")})
}
func (app *app) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if app.projectRepo == nil || app.projectRepo.DB.PingContext(ctx) != nil {
		http.Error(w, "Database is not ready", 503)
		return
	}
	var ready int
	if err := app.projectRepo.DB.QueryRowContext(ctx, `SELECT 1 FROM goose_db_version LIMIT 1`).Scan(&ready); err != nil {
		http.Error(w, "Migrations are not ready", 503)
		return
	}
	writeJSON(w, 200, map[string]any{"ready": true, "preview": previewMode(), "previewProtocol": PreviewProtocolVersion})
}

func previewGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		jobWrite := r.Method != http.MethodGet && (strings.Contains(path, "/automation/jobs") || (strings.Contains(path, "/automation/templates/") && strings.HasSuffix(path, "/job")))
		if previewMode() && (strings.HasPrefix(path, "/api/ai/") || jobWrite || strings.HasPrefix(path, "/api/environment") || strings.Contains(path, "/environments") || strings.HasSuffix(path, "/request-agent") || (r.Method != "GET" && strings.Contains(path, "/conductor"))) {
			http.Error(w, "AI execution, repository operations, and environment management are disabled inside application previews.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// A preview's JavaScript must not use the browser to control the main app on a
// different localhost port. Dev frontends can explicitly opt in their origin.
func trustedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	if u.Host == r.Host {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("AYCORN_ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}

// Seed only an empty preview database. Restarts preserve all review edits.
func seedPreview(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM project`).Scan(&count); err != nil || count > 0 {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	workflow, err := tx.Exec(`INSERT INTO workflow(name) VALUES('Preview workflow')`)
	if err != nil {
		return err
	}
	workflowID, _ := workflow.LastInsertId()
	for i, stage := range []struct{ name, kind string }{{"Backlog", "open"}, {"Planning", "todo"}, {"Doing", "doing"}, {"In review", "todo"}, {"Done", "done"}} {
		if _, err = tx.Exec(`INSERT INTO stage(workflow,name,type,color,icon,position) VALUES(?,?,?,'gray','circle',?)`, workflowID, stage.name, stage.kind, i+1); err != nil {
			return err
		}
	}
	project, err := tx.Exec(`INSERT INTO project(name,pinned,workflow,defaultView) VALUES('Preview sandbox',false,?,'kanban')`, workflowID)
	if err != nil {
		return err
	}
	projectID, _ := project.LastInsertId()
	checklist, err := tx.Exec(`INSERT INTO checklist(project,name,isDefault) VALUES(?,'Sample tasks',1)`, projectID)
	if err != nil {
		return err
	}
	checklistID, _ := checklist.LastInsertId()
	_, err = tx.Exec(`INSERT INTO task(checklist,stage,type,name,body,priority) SELECT ?,s.id,tt.id,'Explore this branch', '[{"type":"p","children":[{"text":"This is isolated preview data. Try changes here without affecting the main application."}]}]','Medium' FROM stage s CROSS JOIN task_type tt WHERE s.workflow=? AND s.type='open' ORDER BY tt.isDefault DESC,tt.id LIMIT 1`, checklistID, workflowID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
