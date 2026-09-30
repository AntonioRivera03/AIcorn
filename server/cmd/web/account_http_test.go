package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/accounts"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
)

// capturingMailer keeps sent emails so a test can follow their links.
type capturingMailer struct {
	mu   sync.Mutex
	sent []accounts.Email
}

func (m *capturingMailer) Send(_ context.Context, email accounts.Email) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, email)
	return nil
}

// lastToken returns the token in the link of the latest email.
func (m *capturingMailer) lastToken(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no email sent")
	}
	_, after, ok := strings.Cut(m.sent[len(m.sent)-1].Text, "token=")
	if !ok {
		t.Fatalf("no link in %q", m.sent[len(m.sent)-1].Text)
	}
	return strings.Fields(after)[0]
}

func TestUnconfirmedEmailCantReachWorkspaces(t *testing.T) {
	mail := &capturingMailer{}
	srv, _ := testServerWith(t, func(s *server) {
		s.accounts.Mailer = mail
		s.accounts.VerifyEmails = true
	})
	ada := newClient(t, srv)
	me := ada.signup("Ada", "ada@example.com")
	if me.Account.EmailVerified {
		t.Fatal("new account should start unconfirmed")
	}
	personal := me.Workspaces[0].ID
	ada.do("GET", "/api/project", personal, "", 403)
	ada.do("POST", "/api/workspaces", 0, `{"name":"Acme"}`, 403)
	// Signed-in account endpoints still work, so the app can show the
	// confirm-your-email page.
	ada.do("GET", "/api/auth/me", 0, "", 200)

	ada.do("POST", "/api/auth/verify-email/resend", 0, "", 204)
	if len(mail.sent) != 2 {
		t.Fatalf("expected the signup email and a resend, got %d emails", len(mail.sent))
	}
	// The link works in any browser, signed in or not.
	newClient(t, srv).do("POST", "/api/auth/verify-email", 0, `{"token":"`+mail.lastToken(t)+`"}`, 204)
	ada.do("GET", "/api/project", personal, "", 200)
	ada.do("POST", "/api/auth/verify-email", 0, `{"token":"`+mail.lastToken(t)+`"}`, 400)
}

func TestPasswordResetAndChangeOverHTTP(t *testing.T) {
	mail := &capturingMailer{}
	srv, _ := testServerWith(t, func(s *server) { s.accounts.Mailer = mail })
	ada := newClient(t, srv)
	ada.signup("Ada", "ada@example.com")

	elsewhere := newClient(t, srv)
	var sent struct{ EmailSent bool }
	json.Unmarshal(elsewhere.do("POST", "/api/auth/password-reset", 0, `{"email":"ada@example.com"}`, 200), &sent)
	if !sent.EmailSent {
		t.Fatal("reset should report the email as sent")
	}
	elsewhere.do("POST", "/api/auth/password-reset/confirm", 0, `{"token":"`+mail.lastToken(t)+`","password":"battery staple"}`, 200)
	elsewhere.do("GET", "/api/auth/me", 0, "", 200)
	ada.do("GET", "/api/auth/me", 0, "", 401)

	elsewhere.do("PUT", "/api/auth/me/password", 0, `{"current":"correct horse","new":"horse battery"}`, 400)
	elsewhere.do("PUT", "/api/auth/me/password", 0, `{"current":"battery staple","new":"horse battery"}`, 204)
	// The session that changed the password stays signed in.
	elsewhere.do("GET", "/api/auth/me", 0, "", 200)
	ada.do("POST", "/api/auth/login", 0, `{"email":"ada@example.com","password":"horse battery"}`, 200)
}

func TestLoginIsRateLimited(t *testing.T) {
	srv, _ := testServer(t)
	newClient(t, srv).signup("Ada", "ada@example.com")
	newClient(t, srv).signup("Bea", "bea@example.com")

	guesser := newClient(t, srv)
	for range 10 {
		guesser.do("POST", "/api/auth/login", 0, `{"email":"ada@example.com","password":"wrong password"}`, 401)
	}
	// Once an account has had too many wrong passwords, even the right one
	// waits, whatever the email's case.
	_, header := guesser.send("POST", "/api/auth/login", 0, `{"email":"ADA@example.com","password":"correct horse"}`, 429)
	if header.Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After")
	}
	// Other accounts aren't affected.
	guesser.do("POST", "/api/auth/login", 0, `{"email":"bea@example.com","password":"correct horse"}`, 200)
}

func TestSignupIsRateLimitedPerAddress(t *testing.T) {
	srv, _ := testServerWith(t, func(s *server) { s.limits.perIP = newRateLimiter(2, time.Minute) })
	c := newClient(t, srv)
	c.signup("Ada", "ada@example.com")
	c.signup("Bea", "bea@example.com")
	c.do("POST", "/api/auth/signup", 0, `{"name":"Cy","email":"cy@example.com","password":"correct horse"}`, 429)
}

func TestLosingAccessIsFlagged(t *testing.T) {
	var dataDir string
	srv, _ := testServerWith(t, func(s *server) { dataDir = s.workspaces.dataDir })
	owner := newClient(t, srv)
	ownerMe := owner.signup("Owner", "owner@example.com")
	var org accounts.Workspace
	json.Unmarshal(owner.do("POST", "/api/workspaces", 0, `{"name":"Acme"}`, 201), &org)
	orgPath := "/api/workspaces/" + strconv.FormatInt(org.ID, 10)

	join := func(name, email, role string) (*client, int64) {
		var invite accounts.CreatedInvite
		json.Unmarshal(owner.do("POST", orgPath+"/invites", 0, `{"email":"`+email+`","role":"`+role+`"}`, 201), &invite)
		c := newClient(t, srv)
		me := c.signup(name, email)
		c.do("POST", "/api/invites/accept", 0, `{"code":"`+invite.Code+`"}`, 200)
		return c, me.Account.ID
	}
	bea, beaID := join("Bea", "bea@example.com", "member")
	cy, _ := join("Cy", "cy@example.com", "admin")

	// Removed while the organization is open: the next request says so.
	bea.do("GET", "/api/project", org.ID, "", 200)
	owner.do("DELETE", orgPath+"/members/"+strconv.FormatInt(beaID, 10), 0, "", 204)
	if _, header := bea.send("GET", "/api/project", org.ID, "", 404); header.Get(workspaceAccessHeader) != "none" {
		t.Fatalf("removed member's 404 isn't flagged: %v", header)
	}
	// An ordinary 404 inside a workspace isn't.
	if _, header := owner.send("GET", "/api/task/999999", org.ID, "", 404); header.Get(workspaceAccessHeader) != "" {
		t.Fatal("a missing task was flagged as lost access")
	}

	// Only owners delete organizations, and personal workspaces stay.
	cy.do("DELETE", orgPath, 0, "", 403)
	owner.do("DELETE", "/api/workspaces/"+strconv.FormatInt(ownerMe.Workspaces[0].ID, 10), 0, "", 400)

	owner.do("DELETE", orgPath, 0, "", 204)
	for _, c := range []*client{owner, cy} {
		if _, header := c.send("GET", "/api/project", org.ID, "", 404); header.Get(workspaceAccessHeader) != "none" {
			t.Fatal("deleted organization's 404 isn't flagged")
		}
	}
	// Its data is set aside, not left where a new workspace could find it.
	if _, err := os.Stat(filepath.Dir(appdb.WorkspaceDBPath(dataDir, org.ID))); !os.IsNotExist(err) {
		t.Fatalf("deleted workspace's directory is still in place: %v", err)
	}
	trash, err := os.ReadDir(filepath.Join(dataDir, "deleted-workspaces"))
	if err != nil || len(trash) != 1 || !strings.HasPrefix(trash[0].Name(), strconv.FormatInt(org.ID, 10)+"-") {
		t.Fatalf("deleted workspace wasn't set aside: %v %v", err, trash)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "deleted-workspaces", trash[0].Name(), "app.db")); err != nil {
		t.Fatalf("set-aside workspace lost its database: %v", err)
	}
}

func TestPreviewSignsVisitorsIn(t *testing.T) {
	srv, _ := testServerWith(t, func(s *server) { s.previewSignIn = true })
	var first, second meResponse
	json.Unmarshal(newClient(t, srv).do("GET", "/api/auth/me", 0, "", 200), &first)
	json.Unmarshal(newClient(t, srv).do("GET", "/api/auth/me", 0, "", 200), &second)
	if first.Account.ID == 0 || first.Account.ID != second.Account.ID {
		t.Fatalf("visitors should share one reviewer account: %+v %+v", first.Account, second.Account)
	}
	if !first.Account.EmailVerified || first.Account.Usage != accounts.UsageSolo || len(first.Workspaces) != 1 {
		t.Fatalf("reviewer account should be ready to use: %+v", first)
	}
}
