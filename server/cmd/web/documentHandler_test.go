package main

import (
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentsHTTP(t *testing.T) {
	a := aiTestApp(t, "")
	a.projectRepo = a.aiService.Projects
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == "DELETE" {
			req.Header.Set("If-Match", "2")
		}
		a.routes().ServeHTTP(r, req)
		if r.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	base := "/api/documents/project/1"
	r := call("POST", base, "", 201)
	var d knowledge.Document
	if err := json.Unmarshal(r.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	item := base + "/" + fmtInt(d.ID)
	call("PUT", item, `{"revision":1,"title":"Decisions"}`, 200)
	call("PUT", item, `{"revision":1,"title":"Stale"}`, 409)
	call("PUT", item, `{"revision":2,"body":[{}]}`, 400)
	call("PUT", item, `{"revision":2,"unknown":true}`, 400)
	call("GET", base, "", 200)
	call("GET", "/api/documents/project/999/"+fmtInt(d.ID), "", 404)
	call("DELETE", item, "", 204)
	call("GET", item, "", 404)
}
