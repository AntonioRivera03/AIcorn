package main

import (
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
	"net/http"
)

func (a *app) projectChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/project-chats/project/{projectId}", a.projectChatHistory)
	mux.HandleFunc("GET /api/project-chats/project/{projectId}/tasks", a.projectChatTasks)
	mux.HandleFunc("POST /api/project-chats/project/{projectId}/messages", a.projectChatSend)
	mux.HandleFunc("POST /api/project-chats/project/{projectId}/{id}/cancel", a.projectChatCancel)
}
func (a *app) projectChatHistory(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	c, err := a.projectChatService.Conversation(project)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}
func (a *app) projectChatSend(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	var in projectchat.Input
	if !decodeJSONInput(w, r, &in) {
		return
	}
	t, err := a.projectChatService.Send(r.Context(), project, in)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 202, t)
}
func (a *app) projectChatCancel(w http.ResponseWriter, r *http.Request) {
	project, id, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	if err := a.projectChatService.Cancel(project, id); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) projectChatTasks(w http.ResponseWriter, r *http.Request) {
	project, _, ok := jobRouteIDs(w, r)
	if !ok {
		return
	}
	if _, err := a.projectRepo.FindOne(project); err != nil {
		respondErr(w, err)
		return
	}
	rows, err := a.projectRepo.DB.Query("SELECT t.id,t.name FROM task t JOIN checklist c ON c.id=t.checklist WHERE c.project=? ORDER BY t.id DESC", project)
	if err != nil {
		respondErr(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}
	items := []item{}
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.ID, &i.Title); err != nil {
			respondErr(w, err)
			return
		}
		items = append(items, i)
	}
	if err = rows.Err(); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, items)
}
