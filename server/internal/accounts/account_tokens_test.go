package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// linkToken pulls the token out of the one-time link in an email.
func linkToken(t *testing.T, email Email) string {
	t.Helper()
	_, after, ok := strings.Cut(email.Text, "token=")
	if !ok {
		t.Fatalf("no link in email %q", email.Text)
	}
	return strings.Fields(after)[0]
}

func (m *recordingMailer) last(t *testing.T) Email {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no email sent")
	}
	return m.sent[len(m.sent)-1]
}

func TestSignupWithoutVerificationStartsVerified(t *testing.T) {
	svc, mailer, _ := newTestService(t)
	account := mustSignup(t, svc, "Ada", "ada@example.com")
	if !account.EmailVerified {
		t.Fatal("with verification off, new accounts should start verified")
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("no confirmation email expected, got %+v", mailer.sent)
	}
}

func TestEmailVerification(t *testing.T) {
	svc, mailer, _ := newTestService(t)
	svc.VerifyEmails = true
	ctx := context.Background()

	account, session, err := svc.Signup(ctx, SignupInput{Name: "Ada", Email: "ada@example.com", Password: "correct horse"}, "https://aycorn.example/")
	if err != nil {
		t.Fatal(err)
	}
	if account.EmailVerified {
		t.Fatal("account should start unverified")
	}
	sent := mailer.last(t)
	if sent.To != "ada@example.com" || !strings.Contains(sent.Text, "https://aycorn.example/verify-email?token=") {
		t.Fatalf("unexpected confirmation email %+v", sent)
	}
	first := linkToken(t, sent)

	// Resending replaces the earlier link.
	if err := svc.SendVerification(ctx, account.ID, "https://aycorn.example"); err != nil {
		t.Fatal(err)
	}
	second := linkToken(t, mailer.last(t))
	if _, err := svc.VerifyEmail(ctx, first); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("replaced link still works: %v", err)
	}

	verified, err := svc.VerifyEmail(ctx, second)
	if err != nil || !verified.EmailVerified || verified.ID != account.ID {
		t.Fatalf("verify: %v %+v", err, verified)
	}
	authed, _, err := svc.Authenticate(ctx, session.Token)
	if err != nil || !authed.EmailVerified {
		t.Fatalf("session should see the verified account: %v %+v", err, authed)
	}
	if _, err := svc.VerifyEmail(ctx, second); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("link reused: %v", err)
	}

	// Nothing is sent to an already verified account.
	before := len(mailer.sent)
	if err := svc.SendVerification(ctx, account.ID, ""); err != nil || len(mailer.sent) != before {
		t.Fatalf("resend to a verified account: %v, %d emails", err, len(mailer.sent)-before)
	}
}

func TestVerificationLinksExpire(t *testing.T) {
	svc, mailer, _ := newTestService(t)
	svc.VerifyEmails = true
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	mustSignup(t, svc, "Ada", "ada@example.com")
	token := linkToken(t, mailer.last(t))
	now = now.Add(49 * time.Hour)
	if _, err := svc.VerifyEmail(context.Background(), token); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired link worked: %v", err)
	}
}

func TestInvitesProveEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner := mustSignup(t, svc, "Owner", "owner@example.com")
	svc.VerifyEmails = true
	org, _ := svc.CreateOrganization(ctx, owner.ID, "Acme")
	beaInvite, _ := svc.Invite(ctx, owner.ID, org.ID, "bea@example.com", RoleMember, "")
	cyInvite, _ := svc.Invite(ctx, owner.ID, org.ID, "cy@example.com", RoleMember, "")

	// Signing up from an invite sent to your address skips confirmation…
	bea, _, err := svc.Signup(ctx, SignupInput{Name: "Bea", Email: "bea@example.com", Password: "correct horse", InviteCode: beaInvite.Code}, "")
	if err != nil || !bea.EmailVerified {
		t.Fatalf("invited signup should start verified: %v %+v", err, bea)
	}
	// …but someone else's invite code proves nothing about your address.
	eve, _, err := svc.Signup(ctx, SignupInput{Name: "Eve", Email: "eve@example.com", Password: "correct horse", InviteCode: cyInvite.Code}, "")
	if err != nil || eve.EmailVerified {
		t.Fatalf("another address's invite verified the account: %v %+v", err, eve)
	}

	// Accepting an invite after signing up proves the address too.
	cy := mustSignup(t, svc, "Cy", "cy@example.com")
	if cy.EmailVerified {
		t.Fatal("plain signup should start unverified")
	}
	if _, err := svc.AcceptInvite(ctx, cy.ID, cyInvite.Code); err != nil {
		t.Fatal(err)
	}
	if cy, _ = svc.Store.AccountByID(ctx, cy.ID); !cy.EmailVerified {
		t.Fatal("accepting an invite should verify the email")
	}
}

func TestPasswordReset(t *testing.T) {
	svc, mailer, _ := newTestService(t)
	ctx := context.Background()
	account := mustSignup(t, svc, "Ada", "ada@example.com")
	_, oldSession, err := svc.Login(ctx, "ada@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}

	// Unknown addresses get the same answer and no email.
	if sent, err := svc.RequestPasswordReset(ctx, "nobody@example.com", ""); err != nil || !sent || len(mailer.sent) != 0 {
		t.Fatalf("unknown email: %v %v %d", sent, err, len(mailer.sent))
	}
	if sent, err := svc.RequestPasswordReset(ctx, " ADA@example.com ", "https://aycorn.example"); err != nil || !sent {
		t.Fatalf("request reset: %v %v", sent, err)
	}
	token := linkToken(t, mailer.last(t))

	if _, _, err := svc.ResetPassword(ctx, token, "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("short password accepted: %v", err)
	}
	reset, session, err := svc.ResetPassword(ctx, token, "battery staple")
	if err != nil || reset.ID != account.ID || session.Token == "" {
		t.Fatalf("reset: %v %+v", err, reset)
	}
	if _, _, err := svc.Authenticate(ctx, oldSession.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old sessions should end on reset: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, session.Token); err != nil {
		t.Fatalf("reset should sign in: %v", err)
	}
	if _, _, err := svc.Login(ctx, "ada@example.com", "battery staple"); err != nil {
		t.Fatalf("new password: %v", err)
	}
	if _, _, err := svc.ResetPassword(ctx, token, "another password"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("reset link reused: %v", err)
	}
}

func TestPasswordResetWithoutEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.Mailer = LogMailer{}
	mustSignup(t, svc, "Ada", "ada@example.com")
	for _, email := range []string{"ada@example.com", "nobody@example.com"} {
		if sent, err := svc.RequestPasswordReset(context.Background(), email, ""); err != nil || sent {
			t.Fatalf("%s: without a mailer the answer should be 'not sent', got %v %v", email, sent, err)
		}
	}
}

func TestChangePassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	account := mustSignup(t, svc, "Ada", "ada@example.com")
	_, here, _ := svc.Login(ctx, "ada@example.com", "correct horse")
	_, elsewhere, _ := svc.Login(ctx, "ada@example.com", "correct horse")

	if err := svc.ChangePassword(ctx, account.ID, here.Token, "wrong password", "battery staple"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("wrong current password: %v", err)
	}
	if err := svc.ChangePassword(ctx, account.ID, here.Token, "correct horse", "battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, here.Token); err != nil {
		t.Fatalf("the session that changed the password should stay signed in: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, elsewhere.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("other sessions should end: %v", err)
	}
	if _, _, err := svc.Login(ctx, "ada@example.com", "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
}

func TestPreviewSession(t *testing.T) {
	svc, _, provisioned := newTestService(t)
	svc.VerifyEmails = true
	ctx := context.Background()
	first, session, err := svc.PreviewSession(ctx, "Preview reviewer", "preview@aycorn.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if !first.EmailVerified || first.Usage != UsageSolo || session.Token == "" {
		t.Fatalf("preview account should be ready to use: %+v", first)
	}
	again, _, err := svc.PreviewSession(ctx, "Preview reviewer", "preview@aycorn.invalid")
	if err != nil || again.ID != first.ID {
		t.Fatalf("preview account should be reused: %v %+v", err, again)
	}
	if len(*provisioned) != 1 {
		t.Fatalf("expected one personal workspace, provisioned %v", *provisioned)
	}
}
