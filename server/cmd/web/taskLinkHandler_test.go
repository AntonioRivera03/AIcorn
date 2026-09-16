package main

import (
	"encoding/json"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaskLinksHTTP(t *testing.T) {
	a := aiTestApp(t, "")
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("If-Match", "2")
		a.routes().ServeHTTP(r, req)
		if r.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	base := "/api/task-links/task/1"
	r := call("POST", base, `{"url":"https://github.com/owner/repo/pull/15","label":"Feature"}`, 201)
	var link knowledge.TaskLink
	if err := json.Unmarshal(r.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	item := base + "/" + fmtInt(link.ID)
	call("POST", base, `{"url":"https://github.com/OWNER/REPO/pull/15"}`, 201)
	call("PUT", item, `{"url":"https://github.com/owner/repo/tree/feature","label":"Branch","revision":1}`, 200)
	call("PUT", item, `{"url":"https://github.com/owner/repo/tree/feature","revision":1}`, 409)
	call("POST", base, `{"url":"javascript:alert(1)"}`, 400)
	call("PUT", item, `{"url":"https://github.com/owner/repo/tree/feature","revision":2,"taskId":99}`, 400)
	call("GET", base, "", 200)
	call("POST", "/api/task-links/task/999", `{"url":"https://github.com/owner/repo/pull/1"}`, 404)
	call("DELETE", "/api/task-links/task/999/"+fmtInt(link.ID), "", 404)
	call("DELETE", item, "", 204)
	call("DELETE", item, "", 404)
}
