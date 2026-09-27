package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/accounts"
)

// testServer runs the full multi-user server (accounts DB plus real
// per-workspace runtimes) against a temp data directory.
func testServer(t *testing.T) (*httptest.Server, *accounts.Service) {
	t.Helper()
	// Preview mode keeps workspace runtimes from starting agent workers,
	// schedulers, and Kubernetes environments.
	t.Setenv("AYCORN_PREVIEW", "1")
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	accountsDB, err := accounts.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := newWorkspaceRegistry(ctx, dataDir, runtimeConfig{})
	service := &accounts.Service{Store: accounts.NewStore(accountsDB), Mailer: accounts.LogMailer{}, Provision: workspaces.provision, Unprovision: workspaces.unprovision}
	srv := httptest.NewServer((&server{accounts: service, workspaces: workspaces, accountsDB: accountsDB}).routes())
	t.Cleanup(func() {
		srv.Close()
		cancel()
		workspaces.stopAll()
		accountsDB.Close()
	})
	return srv, service
}

// client is one signed-in browser.
type client struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, srv: srv, c: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, workspace int64, body string, want int) []byte {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if workspace != 0 {
		req.Header.Set(workspaceHeader, strconv.FormatInt(workspace, 10))
	}
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.StatusCode, want, b)
	}
	return b
}

func (c *client) signup(name, email string) meResponse {
	c.t.Helper()
	var me meResponse
	body := `{"name":"` + name + `","email":"` + email + `","password":"correct horse"}`
	if err := json.Unmarshal(c.do("POST", "/api/auth/signup", 0, body, 201), &me); err != nil {
		c.t.Fatal(err)
	}
	return me
}

func projectNames(t *testing.T, raw []byte) []string {
	t.Helper()
	var projects []struct{ Name string }
	if err := json.Unmarshal(raw, &projects); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return names
}

func TestWorkspaceAPIRequiresSessionAndMembership(t *testing.T) {
	srv, _ := testServer(t)
	anon := newClient(t, srv)
	anon.do("GET", "/api/project", 1, "", 401)
	anon.do("GET", "/api/auth/me", 0, "", 401)

	ada := newClient(t, srv)
	adaMe := ada.signup("Ada", "ada@example.com")
	if len(adaMe.Workspaces) != 1 || adaMe.Workspaces[0].Kind != accounts.KindPersonal {
		t.Fatalf("expected a personal workspace: %+v", adaMe.Workspaces)
	}
	adaPersonal := adaMe.Workspaces[0].ID

	// The workspace header is required, so a request can't silently land in
	// the wrong workspace.
	ada.do("GET", "/api/project", 0, "", 400)

	// Each workspace starts with a workflow (here the preview seed; otherwise
	// the starter workflow) plus the task types the migrations seed.
	var workflows []struct{ ID int }
	json.Unmarshal(ada.do("GET", "/api/workflow", adaPersonal, "", 200), &workflows)
	if len(workflows) == 0 {
		t.Fatal("new workspace has no seeded workflows")
	}
	ada.do("POST", "/api/project", adaPersonal, `{"workflowId":`+strconv.Itoa(workflows[0].ID)+`}`, 200)

	bob := newClient(t, srv)
	bobMe := bob.signup("Bob", "bob@example.com")
	bobPersonal := bobMe.Workspaces[0].ID
	if names := projectNames(t, bob.do("GET", "/api/project", bobPersonal, "", 200)); len(names) != 1 || names[0] != "Preview sandbox" {
		t.Fatalf("Bob sees another workspace's projects: %v", names)
	}
	// Bob can't reach Ada's workspace by guessing its ID.
	bob.do("GET", "/api/project", adaPersonal, "", 404)

	// Logging out ends the session.
	ada.do("POST", "/api/auth/logout", 0, "", 204)
	ada.do("GET", "/api/project", adaPersonal, "", 401)
	ada.do("POST", "/api/auth/login", 0, `{"email":"ada@example.com","password":"correct horse"}`, 200)
	if names := projectNames(t, ada.do("GET", "/api/project", adaPersonal, "", 200)); len(names) != 2 {
		t.Fatalf("Ada lost her project: %v", names)
	}
}

func TestOrganizationInviteOverHTTP(t *testing.T) {
	srv, _ := testServer(t)
	owner := newClient(t, srv)
	owner.signup("Owner", "owner@example.com")
	var org accounts.Workspace
	json.Unmarshal(owner.do("POST", "/api/workspaces", 0, `{"name":"Acme"}`, 201), &org)
	orgPath := "/api/workspaces/" + strconv.FormatInt(org.ID, 10)

	var invite accounts.CreatedInvite
	json.Unmarshal(owner.do("POST", orgPath+"/invites", 0, `{"email":"bea@example.com"}`, 201), &invite)
	if invite.Code == "" || invite.EmailSent || invite.EmailError == "" {
		t.Fatalf("without Resend the inviter should get the code to share: %+v", invite)
	}

	anon := newClient(t, srv)
	var preview accounts.InvitePreview
	json.Unmarshal(anon.do("GET", "/api/invites/"+invite.Code, 0, "", 200), &preview)
	if preview.WorkspaceName != "Acme" {
		t.Fatalf("preview: %+v", preview)
	}
	anon.do("GET", "/api/invites/NOPE-NOPE", 0, "", 404)

	bea := newClient(t, srv)
	bea.signup("Bea", "bea@example.com")
	bea.do("GET", "/api/project", org.ID, "", 404)
	bea.do("POST", "/api/invites/accept", 0, `{"code":"`+invite.Code+`"}`, 200)
	bea.do("GET", "/api/project", org.ID, "", 200)

	var members []accounts.Member
	json.Unmarshal(bea.do("GET", orgPath+"/members", 0, "", 200), &members)
	if len(members) != 2 {
		t.Fatalf("members: %+v", members)
	}
	// Members can't invite or see pending invites.
	bea.do("POST", orgPath+"/invites", 0, `{"email":"cy@example.com"}`, 403)
	bea.do("GET", orgPath+"/invites", 0, "", 403)
}

func TestForeignOriginsAreRejected(t *testing.T) {
	srv, _ := testServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://127.0.0.1:49200")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 || res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign origin reached the API: %d", res.StatusCode)
	}
}

func TestUnprovisionLeavesNothingForAReusedID(t *testing.T) {
	t.Setenv("AYCORN_PREVIEW", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := newWorkspaceRegistry(ctx, t.TempDir(), runtimeConfig{})
	defer registry.stopAll()

	if err := registry.provision(ctx, 7); err != nil {
		t.Fatal(err)
	}
	rt, err := registry.get(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.app.projectService.CreateProject(1); err != nil {
		t.Fatal(err)
	}

	registry.unprovision(7)
	if _, err := os.Stat(filepath.Dir(registry.dbPath(7))); !os.IsNotExist(err) {
		t.Fatalf("workspace directory survived unprovision: %v", err)
	}

	// A new workspace given the same ID starts from an empty database.
	fresh, err := registry.get(7)
	if err != nil {
		t.Fatal(err)
	}
	if fresh == rt {
		t.Fatal("registry handed back the rolled-back runtime")
	}
	projects, err := fresh.app.projectRepo.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.Name != "Preview sandbox" {
			t.Fatalf("rolled-back workspace's data leaked into its successor: %+v", projects)
		}
	}
}

func TestConcurrentGetsShareOneRuntime(t *testing.T) {
	t.Setenv("AYCORN_PREVIEW", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := newWorkspaceRegistry(ctx, t.TempDir(), runtimeConfig{})
	defer registry.stopAll()

	results := make(chan *workspaceRuntime, 8)
	for range 8 {
		go func() {
			rt, err := registry.get(3)
			if err != nil {
				t.Error(err)
			}
			results <- rt
		}()
	}
	first := <-results
	for range 7 {
		if rt := <-results; rt != first {
			t.Fatal("concurrent gets started more than one runtime for a workspace")
		}
	}
}
