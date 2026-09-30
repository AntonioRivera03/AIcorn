package accounts

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type recordingMailer struct {
	mu   sync.Mutex
	sent []Email
}

func (m *recordingMailer) Send(_ context.Context, email Email) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, email)
	return nil
}

func newTestService(t *testing.T) (*Service, *recordingMailer, *[]int64) {
	t.Helper()
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	mailer := &recordingMailer{}
	var provisioned []int64
	svc := &Service{
		Store:  NewStore(db),
		Mailer: mailer,
		Provision: func(_ context.Context, id int64) error {
			provisioned = append(provisioned, id)
			return nil
		},
	}
	return svc, mailer, &provisioned
}

func mustSignup(t *testing.T, svc *Service, name, email string) Account {
	t.Helper()
	account, _, err := svc.Signup(context.Background(), SignupInput{Name: name, Email: email, Password: "correct horse"}, "")
	if err != nil {
		t.Fatalf("signup %s: %v", email, err)
	}
	return account
}

func TestSignupCreatesPersonalWorkspaceAndSession(t *testing.T) {
	svc, _, provisioned := newTestService(t)
	ctx := context.Background()

	account, session, err := svc.Signup(ctx, SignupInput{Name: " Ada ", Email: "ada@example.com", Password: "correct horse"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "Ada" || account.Usage != "" {
		t.Fatalf("unexpected account %+v", account)
	}
	workspaces, err := svc.Workspaces(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].Kind != KindPersonal || workspaces[0].Role != RoleOwner {
		t.Fatalf("expected one personal workspace, got %+v", workspaces)
	}
	if len(*provisioned) != 1 || (*provisioned)[0] != workspaces[0].ID {
		t.Fatalf("personal workspace not provisioned: %v", *provisioned)
	}

	authed, _, err := svc.Authenticate(ctx, session.Token)
	if err != nil || authed.ID != account.ID {
		t.Fatalf("session did not authenticate: %v %+v", err, authed)
	}

	if _, _, err := svc.Signup(ctx, SignupInput{Name: "Ada 2", Email: "ADA@example.com", Password: "correct horse"}, ""); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email (any case) should be rejected, got %v", err)
	}
}

func TestSignupValidation(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	cases := []struct{ name, email, password string }{
		{"", "a@example.com", "correct horse"},
		{"A", "not-an-email", "correct horse"},
		{"A", "a@example.com", "short"},
	}
	for _, c := range cases {
		if _, _, err := svc.Signup(ctx, SignupInput{Name: c.name, Email: c.email, Password: c.password}, ""); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("signup(%q, %q, %q) = %v, want ErrInvalidInput", c.name, c.email, c.password, err)
		}
	}
}

func TestLogin(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	mustSignup(t, svc, "Ada", "ada@example.com")

	if _, _, err := svc.Login(ctx, "ada@example.com", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, err := svc.Login(ctx, "nobody@example.com", "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown email: %v", err)
	}
	_, session, err := svc.Login(ctx, "Ada@Example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("logged-out session still works: %v", err)
	}
}

func TestSessionExpiryAndRenewal(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	mustSignup(t, svc, "Ada", "ada@example.com")
	_, session, err := svc.Login(ctx, "ada@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(20 * 24 * time.Hour)
	_, renewed, err := svc.Authenticate(ctx, session.Token)
	if err != nil || renewed == nil {
		t.Fatalf("a session near expiry should renew: %v %v", err, renewed)
	}

	now = now.Add(31 * 24 * time.Hour)
	if _, _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session still works: %v", err)
	}
}

func TestInviteFlow(t *testing.T) {
	svc, mailer, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Acme")
	if err != nil {
		t.Fatal(err)
	}

	invite, err := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, "http://localhost:5173/")
	if err != nil {
		t.Fatal(err)
	}
	if !invite.EmailSent || len(mailer.sent) != 1 || mailer.sent[0].To != "bea@example.com" {
		t.Fatalf("invite email not sent: %+v %+v", invite, mailer.sent)
	}
	if invite.Link != "http://localhost:5173/invite/"+invite.Code {
		t.Fatalf("unexpected link %q", invite.Link)
	}

	preview, err := svc.PreviewInvite(ctx, invite.Code)
	if err != nil || preview.WorkspaceName != "Acme" || preview.InvitedBy != "Owner" {
		t.Fatalf("preview: %v %+v", err, preview)
	}

	// Someone else holding the code can't use it.
	eve := mustSignup(t, svc, "Eve", "eve@example.com")
	if _, err := svc.AcceptInvite(ctx, eve.ID, invite.Code); !errors.Is(err, ErrInviteEmail) {
		t.Fatalf("wrong email accepted invite: %v", err)
	}

	bea := mustSignup(t, svc, "Bea", "bea@example.com")
	// Codes are forgiving about case and dashes.
	joined, err := svc.AcceptInvite(ctx, bea.ID, "  "+strings.ToLower(invite.Code)+" ")
	if err != nil {
		t.Fatal(err)
	}
	if joined.ID != org.ID || joined.Role != RoleMember {
		t.Fatalf("unexpected workspace %+v", joined)
	}
	members, err := svc.Members(ctx, owner.ID, org.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %v %+v", err, members)
	}

	// Single use.
	if _, err := svc.AcceptInvite(ctx, bea.ID, invite.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("invite reused: %v", err)
	}
	if _, err := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, ""); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("re-inviting a member: %v", err)
	}
	// Members can't invite.
	if _, err := svc.Invite(ctx, bea.ID, org.ID, "cy@example.com", RoleMember, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member invited: %v", err)
	}
}

func TestReinviteReplacesPendingInvite(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	org, _ := svc.CreateOrganization(ctx, owner.ID, "Acme")

	first, err := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInvite(ctx, first.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("replaced invite still valid: %v", err)
	}
	pending, err := svc.PendingInvites(ctx, owner.ID, org.ID)
	if err != nil || len(pending) != 1 || pending[0].ID != second.ID {
		t.Fatalf("pending: %v %+v", err, pending)
	}
}

func TestInvitesExpire(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	org, _ := svc.CreateOrganization(ctx, owner.ID, "Acme")
	invite, err := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, "")
	if err != nil {
		t.Fatal(err)
	}
	bea := mustSignup(t, svc, "Bea", "bea@example.com")
	now = now.Add(8 * 24 * time.Hour)
	if _, err := svc.AcceptInvite(ctx, bea.ID, invite.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expired invite accepted: %v", err)
	}
}

func TestPersonalWorkspacesCantInvite(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	workspaces, _ := svc.Workspaces(ctx, owner.ID)
	if _, err := svc.Invite(ctx, owner.ID, workspaces[0].ID, "bea@example.com", RoleMember, ""); !errors.Is(err, ErrPersonalWorkspace) {
		t.Fatalf("personal workspace invited: %v", err)
	}
}

func TestOrganizationKeepsAnOwner(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	org, _ := svc.CreateOrganization(ctx, owner.ID, "Acme")

	if err := svc.RemoveMember(ctx, owner.ID, org.ID, owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("sole owner left: %v", err)
	}
	if err := svc.SetMemberRole(ctx, owner.ID, org.ID, owner.ID, RoleMember); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("sole owner demoted: %v", err)
	}

	invite, _ := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, "")
	bea := mustSignup(t, svc, "Bea", "bea@example.com")
	if _, err := svc.AcceptInvite(ctx, bea.ID, invite.Code); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(ctx, bea.ID, org.ID, owner.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member removed owner: %v", err)
	}
	if err := svc.SetMemberRole(ctx, owner.ID, org.ID, bea.ID, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(ctx, owner.ID, org.ID, owner.ID); err != nil {
		t.Fatalf("owner couldn't leave once another owner exists: %v", err)
	}
	if _, err := svc.WorkspaceForMember(ctx, org.ID, owner.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("left member still has access: %v", err)
	}
}

func TestProvisionFailureRollsBackWorkspace(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	var unprovisioned []int64
	svc.Provision = func(context.Context, int64) error { return errors.New("disk full") }
	svc.Unprovision = func(id int64) { unprovisioned = append(unprovisioned, id) }
	if _, err := svc.CreateOrganization(ctx, owner.ID, "Acme"); err == nil {
		t.Fatal("expected provisioning error")
	}
	workspaces, _ := svc.Workspaces(ctx, owner.ID)
	if len(workspaces) != 1 {
		t.Fatalf("half-created workspace left behind: %+v", workspaces)
	}
	// The partially provisioned database is torn down, since SQLite will hand
	// the rolled-back ID to the next workspace.
	if len(unprovisioned) != 1 {
		t.Fatalf("expected the failed workspace to be unprovisioned, got %v", unprovisioned)
	}
	svc.Provision = func(context.Context, int64) error { return nil }
	org, err := svc.CreateOrganization(ctx, owner.ID, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if org.ID != unprovisioned[0] {
		t.Logf("rolled-back ID %d was not reused (got %d); fine either way", unprovisioned[0], org.ID)
	}
}

func TestSuccessfulCreationIsNotUnprovisioned(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.Unprovision = func(id int64) { t.Fatalf("workspace %d unprovisioned after a successful signup", id) }
	mustSignup(t, svc, "Owner", "owner@example.com")
}

func TestDeleteOrganization(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	var retired []int64
	svc.Retire = func(id int64) { retired = append(retired, id) }
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	org, _ := svc.CreateOrganization(ctx, owner.ID, "Acme")
	invite, _ := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleAdmin, "")
	bea := mustSignup(t, svc, "Bea", "bea@example.com")
	if _, err := svc.AcceptInvite(ctx, bea.ID, invite.Code); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteOrganization(ctx, bea.ID, org.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an admin deleted the organization: %v", err)
	}
	personal, _ := svc.Workspaces(ctx, owner.ID)
	if err := svc.DeleteOrganization(ctx, owner.ID, personal[0].ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a personal workspace was deleted: %v", err)
	}
	if len(retired) != 0 {
		t.Fatalf("nothing should be retired yet, got %v", retired)
	}

	if err := svc.DeleteOrganization(ctx, owner.ID, org.ID); err != nil {
		t.Fatal(err)
	}
	if len(retired) != 1 || retired[0] != org.ID {
		t.Fatalf("deleted organization not retired: %v", retired)
	}
	for _, member := range []Account{owner, bea} {
		if _, err := svc.WorkspaceForMember(ctx, org.ID, member.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s still has access to a deleted organization: %v", member.Name, err)
		}
	}
	if ids, _ := svc.AllWorkspaceIDs(ctx); len(ids) != 2 {
		t.Fatalf("expected only the two personal workspaces left, got %v", ids)
	}
}
