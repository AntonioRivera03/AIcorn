package accounts

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	sessionLifetime = 30 * 24 * time.Hour
	// A session is pushed back out to the full lifetime once it's this close
	// to expiring, so active users stay signed in.
	sessionRenewWithin = 15 * 24 * time.Hour
	inviteLifetime     = 7 * 24 * time.Hour
	verifyLifetime     = 48 * time.Hour
	resetLifetime      = time.Hour
	maxNameLength      = 100
)

// Service holds the account and workspace rules. Provision is called inside
// the transaction that creates a workspace, so a workspace row never exists
// without its database; Unprovision undoes it if that transaction then rolls
// back, so no database outlives its row. Retire is called once a deleted
// organization's row is gone, to stop its runtime and set its data aside.
type Service struct {
	Store       *Store
	Mailer      Mailer
	Provision   func(ctx context.Context, workspaceID int64) error
	Unprovision func(workspaceID int64)
	Retire      func(workspaceID int64)
	// VerifyEmails makes new accounts confirm their email address before they
	// can use a workspace. It needs a working Mailer; without one there's no
	// way to send the link, so accounts start out verified.
	VerifyEmails bool
	Now          func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Session is a freshly issued session token (the cookie value).
type Session struct {
	Token   string
	Expires time.Time
}

// --- signup, login, sessions ---

type SignupInput struct {
	Name     string
	Email    string
	Password string
	// InviteCode is the invite the person signed up from, if any. A live
	// invite sent to Email proves the address, so no confirmation is needed.
	InviteCode string
}

// Signup creates an account with its personal workspace and signs it in.
// When emails must be verified, it also sends the confirmation link, built on
// baseURL (the app's public origin).
func (s *Service) Signup(ctx context.Context, in SignupInput, baseURL string) (Account, Session, error) {
	name, email, err := cleanNameAndEmail(in.Name, in.Email)
	if err != nil {
		return Account{}, Session{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return Account{}, Session{}, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return Account{}, Session{}, err
	}
	account, err := s.createAccount(ctx, name, email, hash, func(tx *Store) (bool, error) {
		return s.emailProvenAtSignup(ctx, tx, email, in.InviteCode)
	})
	if err != nil {
		return Account{}, Session{}, err
	}
	session, err := s.startSession(ctx, account.ID)
	if err != nil {
		return Account{}, Session{}, err
	}
	if !account.EmailVerified {
		// The account exists either way; the confirm page can resend.
		if err := s.SendVerification(ctx, account.ID, baseURL); err != nil {
			log.Printf("verification email to %s: %v", email, err)
		}
	}
	return account, session, nil
}

// createAccount inserts an account and its personal workspace. proven runs
// inside the transaction and says whether the email address is already
// proven, so the account starts out verified.
func (s *Service) createAccount(ctx context.Context, name, email, passwordHash string, proven func(tx *Store) (bool, error)) (Account, error) {
	var account Account
	err := s.inWorkspaceTx(ctx, func(tx *Store, provision provisioner) error {
		if taken, err := tx.EmailExists(ctx, email); err != nil {
			return err
		} else if taken {
			return ErrEmailTaken
		}
		verified, err := proven(tx)
		if err != nil {
			return err
		}
		var verifiedAt *time.Time
		if verified {
			now := s.now()
			verifiedAt = &now
		}
		id, err := tx.InsertAccount(ctx, email, name, passwordHash, verifiedAt)
		if err != nil {
			return err
		}
		account = Account{ID: id, Email: email, Name: name, EmailVerified: verified}
		_, err = s.createWorkspace(ctx, tx, provision, KindPersonal, "Personal", id)
		return err
	})
	return account, err
}

// emailProvenAtSignup reports whether a new account's email needs no
// confirmation: verification is off, or the person holds a live invite that
// was emailed to that address.
func (s *Service) emailProvenAtSignup(ctx context.Context, tx *Store, email, inviteCode string) (bool, error) {
	if !s.VerifyEmails {
		return true, nil
	}
	if strings.TrimSpace(inviteCode) == "" {
		return false, nil
	}
	inv, err := tx.PendingInviteByCodeHash(ctx, hashInviteCode(inviteCode), s.now())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(inv.Email, email), nil
}

func (s *Service) Login(ctx context.Context, email, password string) (Account, Session, error) {
	email = strings.TrimSpace(email)
	account, hash, err := s.Store.AccountByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		passwordMatches(dummyPasswordHash, password)
		return Account{}, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Account{}, Session{}, err
	}
	if !passwordMatches(hash, password) {
		return Account{}, Session{}, ErrInvalidCredentials
	}
	session, err := s.startSession(ctx, account.ID)
	return account, session, err
}

func (s *Service) startSession(ctx context.Context, accountID int64) (Session, error) {
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}
	expires := s.now().Add(sessionLifetime)
	if err := s.Store.InsertSession(ctx, hashSecret(token), accountID, expires); err != nil {
		return Session{}, err
	}
	return Session{Token: token, Expires: expires}, nil
}

// Authenticate resolves a session token to its account. When the session was
// renewed, the returned Session is non-nil and the cookie should be reissued.
func (s *Service) Authenticate(ctx context.Context, token string) (Account, *Session, error) {
	if token == "" {
		return Account{}, nil, ErrUnauthenticated
	}
	tokenHash := hashSecret(token)
	now := s.now()
	account, expires, err := s.Store.SessionAccount(ctx, tokenHash, now)
	if errors.Is(err, ErrNotFound) {
		return Account{}, nil, ErrUnauthenticated
	}
	if err != nil {
		return Account{}, nil, err
	}
	if expires.Sub(now) > sessionRenewWithin {
		return account, nil, nil
	}
	renewed := now.Add(sessionLifetime)
	if err := s.Store.ExtendSession(ctx, tokenHash, renewed); err != nil {
		return Account{}, nil, err
	}
	return account, &Session{Token: token, Expires: renewed}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.Store.DeleteSession(ctx, hashSecret(token))
}

// PruneSessions deletes expired sessions and one-time links; run it
// periodically.
func (s *Service) PruneSessions(ctx context.Context) error {
	return s.Store.DeleteExpired(ctx, s.now())
}

// PreviewSession signs into the account with this email without a password,
// creating it (verified, onboarded as solo) on first use. It exists only for
// application previews: sandboxes reachable only by the people who could
// already use the main app, where nobody should have to sign up. Callers must
// serialize calls, since two first uses would both try to create the account.
func (s *Service) PreviewSession(ctx context.Context, name, email string) (Account, Session, error) {
	account, _, err := s.Store.AccountByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		// Nobody signs in with this password; it only has to be unguessable.
		password, err := newToken()
		if err != nil {
			return Account{}, Session{}, err
		}
		hash, err := hashPassword(password)
		if err != nil {
			return Account{}, Session{}, err
		}
		account, err = s.createAccount(ctx, name, email, hash, func(*Store) (bool, error) { return true, nil })
		if err != nil {
			return Account{}, Session{}, err
		}
		if err := s.Store.SetUsage(ctx, account.ID, UsageSolo); err != nil {
			return Account{}, Session{}, err
		}
		account.Usage = UsageSolo
	} else if err != nil {
		return Account{}, Session{}, err
	}
	session, err := s.startSession(ctx, account.ID)
	return account, session, err
}

// --- email verification ---

// SendVerification emails the account a link that confirms its address,
// replacing any link sent before. It does nothing for a verified account.
func (s *Service) SendVerification(ctx context.Context, accountID int64, baseURL string) error {
	account, err := s.Store.AccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.EmailVerified {
		return nil
	}
	token, err := s.issueToken(ctx, accountID, purposeVerifyEmail, verifyLifetime)
	if err != nil {
		return err
	}
	link := strings.TrimRight(baseURL, "/") + "/verify-email?token=" + token
	return s.Mailer.Send(ctx, verificationEmail(account.Email, account.Name, link))
}

// VerifyEmail confirms the email address of the account the link was sent
// to. Holding the link is the proof, so it works without being signed in.
func (s *Service) VerifyEmail(ctx context.Context, token string) (Account, error) {
	account, err := s.Store.AccountForToken(ctx, hashSecret(token), purposeVerifyEmail, s.now())
	if errors.Is(err, ErrNotFound) {
		return Account{}, ErrTokenInvalid
	}
	if err != nil {
		return Account{}, err
	}
	err = s.Store.InTx(ctx, func(tx *Store) error {
		if err := tx.MarkEmailVerified(ctx, account.ID, s.now()); err != nil {
			return err
		}
		return tx.DeleteTokens(ctx, account.ID, purposeVerifyEmail)
	})
	account.EmailVerified = true
	return account, err
}

// issueToken replaces the account's live link of this purpose with a new one
// and returns its secret.
func (s *Service) issueToken(ctx context.Context, accountID int64, purpose tokenPurpose, lifetime time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	err = s.Store.InTx(ctx, func(tx *Store) error {
		if err := tx.DeleteTokens(ctx, accountID, purpose); err != nil {
			return err
		}
		return tx.InsertToken(ctx, hashSecret(token), accountID, purpose, s.now().Add(lifetime))
	})
	return token, err
}

// --- passwords ---

// RequestPasswordReset emails a password reset link to the account with this
// email, if there is one. The result is the same either way, so the form can't
// be used to find out who has an account. emailSent is false when this server
// can't send email: the link then only reaches the server log.
func (s *Service) RequestPasswordReset(ctx context.Context, email, baseURL string) (emailSent bool, err error) {
	emailSent = EmailEnabled(s.Mailer)
	account, _, err := s.Store.AccountByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, ErrNotFound) {
		return emailSent, nil
	}
	if err != nil {
		return false, err
	}
	token, err := s.issueToken(ctx, account.ID, purposeResetPassword, resetLifetime)
	if err != nil {
		return false, err
	}
	link := strings.TrimRight(baseURL, "/") + "/reset-password?token=" + token
	if err := s.Mailer.Send(ctx, passwordResetEmail(account.Email, account.Name, link)); err != nil && !errors.Is(err, ErrMailerNotConfigured) {
		log.Printf("password reset email to %s: %v", account.Email, err)
	}
	return emailSent, nil
}

// ResetPassword sets a new password from a reset link, signs the account out
// everywhere else, and signs it in here. The link also proves the email.
func (s *Service) ResetPassword(ctx context.Context, token, password string) (Account, Session, error) {
	if err := validatePassword(password); err != nil {
		return Account{}, Session{}, err
	}
	account, err := s.Store.AccountForToken(ctx, hashSecret(token), purposeResetPassword, s.now())
	if errors.Is(err, ErrNotFound) {
		return Account{}, Session{}, ErrTokenInvalid
	}
	if err != nil {
		return Account{}, Session{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Account{}, Session{}, err
	}
	err = s.Store.InTx(ctx, func(tx *Store) error {
		if err := tx.SetPasswordHash(ctx, account.ID, hash); err != nil {
			return err
		}
		if err := tx.MarkEmailVerified(ctx, account.ID, s.now()); err != nil {
			return err
		}
		if err := tx.DeleteTokens(ctx, account.ID, purposeResetPassword); err != nil {
			return err
		}
		return tx.DeleteOtherSessions(ctx, account.ID, "")
	})
	if err != nil {
		return Account{}, Session{}, err
	}
	account.EmailVerified = true
	session, err := s.startSession(ctx, account.ID)
	return account, session, err
}

// ChangePassword replaces the password of a signed-in account and signs it
// out of every other session. currentToken is the session making the change,
// which stays signed in.
func (s *Service) ChangePassword(ctx context.Context, accountID int64, currentToken, current, next string) error {
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := s.Store.PasswordHash(ctx, accountID)
	if err != nil {
		return err
	}
	if !passwordMatches(hash, current) {
		// Not ErrInvalidCredentials: that reads as "signed out" (401).
		return invalidInput("your current password is incorrect")
	}
	newHash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.Store.InTx(ctx, func(tx *Store) error {
		if err := tx.SetPasswordHash(ctx, accountID, newHash); err != nil {
			return err
		}
		if err := tx.DeleteTokens(ctx, accountID, purposeResetPassword); err != nil {
			return err
		}
		return tx.DeleteOtherSessions(ctx, accountID, hashSecret(currentToken))
	})
}

// --- the signed-in account ---

func (s *Service) Workspaces(ctx context.Context, accountID int64) ([]Workspace, error) {
	return s.Store.WorkspacesForAccount(ctx, accountID)
}

func (s *Service) SetUsage(ctx context.Context, accountID int64, usage Usage) error {
	if usage != UsageSolo && usage != UsageOrganization {
		return invalidInput("usage must be solo or organization")
	}
	return s.Store.SetUsage(ctx, accountID, usage)
}

// Rename changes the account's display name and returns the updated account.
func (s *Service) Rename(ctx context.Context, accountID int64, name string) (Account, error) {
	name, err := cleanName(name)
	if err != nil {
		return Account{}, err
	}
	if err := s.Store.SetName(ctx, accountID, name); err != nil {
		return Account{}, err
	}
	return s.Store.AccountByID(ctx, accountID)
}

// --- workspaces ---

// WorkspaceForMember is the authorization check behind every workspace
// request: it fails with ErrNotFound unless accountID belongs to workspaceID.
func (s *Service) WorkspaceForMember(ctx context.Context, workspaceID, accountID int64) (Workspace, error) {
	return s.Store.WorkspaceForMember(ctx, workspaceID, accountID)
}

func (s *Service) AllWorkspaceIDs(ctx context.Context) ([]int64, error) {
	return s.Store.AllWorkspaceIDs(ctx)
}

func (s *Service) CreateOrganization(ctx context.Context, accountID int64, name string) (Workspace, error) {
	name, err := cleanName(name)
	if err != nil {
		return Workspace{}, err
	}
	var workspace Workspace
	err = s.inWorkspaceTx(ctx, func(tx *Store, provision provisioner) error {
		workspace, err = s.createWorkspace(ctx, tx, provision, KindOrganization, name, accountID)
		if err != nil {
			return err
		}
		return tx.SetUsage(ctx, accountID, UsageOrganization)
	})
	return workspace, err
}

// provisioner provisions workspaces created inside one transaction.
type provisioner func(ctx context.Context, workspaceID int64) error

// inWorkspaceTx runs fn in a transaction that may create workspaces. If the
// transaction fails, every workspace provisioned during it is unprovisioned.
func (s *Service) inWorkspaceTx(ctx context.Context, fn func(tx *Store, provision provisioner) error) error {
	var provisioned []int64
	provision := func(ctx context.Context, id int64) error {
		if s.Provision == nil {
			return nil
		}
		// Recorded before provisioning so a partial provision is undone too.
		provisioned = append(provisioned, id)
		return s.Provision(ctx, id)
	}
	err := s.Store.InTx(ctx, func(tx *Store) error { return fn(tx, provision) })
	if err != nil && s.Unprovision != nil {
		for _, id := range provisioned {
			s.Unprovision(id)
		}
	}
	return err
}

// createWorkspace makes the workspace, its owner membership, and its
// database. For a personal workspace, owner is also the workspace's account.
func (s *Service) createWorkspace(ctx context.Context, tx *Store, provision provisioner, kind WorkspaceKind, name string, owner int64) (Workspace, error) {
	var personalAccount *int64
	if kind == KindPersonal {
		personalAccount = &owner
	}
	id, err := tx.InsertWorkspace(ctx, kind, name, personalAccount)
	if err != nil {
		return Workspace{}, err
	}
	if err := tx.InsertMembership(ctx, id, owner, RoleOwner); err != nil {
		return Workspace{}, err
	}
	if err := provision(ctx, id); err != nil {
		return Workspace{}, fmt.Errorf("provisioning workspace %d: %w", id, err)
	}
	return Workspace{ID: id, Kind: kind, Name: name, Role: RoleOwner}, nil
}

// DeleteOrganization deletes an organization for everyone in it. Only owners
// may, and personal workspaces can't be deleted.
func (s *Service) DeleteOrganization(ctx context.Context, actorID, workspaceID int64) error {
	err := s.Store.InTx(ctx, func(tx *Store) error {
		ws, err := tx.WorkspaceForMember(ctx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if ws.Kind != KindOrganization {
			return invalidInput("personal workspaces can't be deleted")
		}
		if ws.Role != RoleOwner {
			return ErrForbidden
		}
		return tx.DeleteWorkspace(ctx, workspaceID)
	})
	if err != nil {
		return err
	}
	if s.Retire != nil {
		s.Retire(workspaceID)
	}
	return nil
}

func (s *Service) RenameWorkspace(ctx context.Context, actorID, workspaceID int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	ws, err := s.Store.WorkspaceForMember(ctx, workspaceID, actorID)
	if err != nil {
		return err
	}
	if !ws.Role.canManageMembers() {
		return ErrForbidden
	}
	return s.Store.RenameWorkspace(ctx, workspaceID, name)
}

// organizationForManager loads an organization the actor may manage.
func (s *Service) organizationForManager(ctx context.Context, actorID, workspaceID int64) (Workspace, error) {
	ws, err := s.Store.WorkspaceForMember(ctx, workspaceID, actorID)
	if err != nil {
		return Workspace{}, err
	}
	if ws.Kind != KindOrganization {
		return Workspace{}, ErrPersonalWorkspace
	}
	if !ws.Role.canManageMembers() {
		return Workspace{}, ErrForbidden
	}
	return ws, nil
}

// --- members ---

func (s *Service) Members(ctx context.Context, actorID, workspaceID int64) ([]Member, error) {
	if _, err := s.Store.WorkspaceForMember(ctx, workspaceID, actorID); err != nil {
		return nil, err
	}
	return s.Store.Members(ctx, workspaceID)
}

// SetMemberRole changes a member's role. Only owners may change roles, and an
// organization always keeps at least one owner.
func (s *Service) SetMemberRole(ctx context.Context, actorID, workspaceID, accountID int64, role Role) error {
	if !validRole(role) {
		return invalidInput("unknown role")
	}
	return s.Store.InTx(ctx, func(tx *Store) error {
		actor, err := tx.WorkspaceForMember(ctx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if actor.Kind != KindOrganization {
			return ErrPersonalWorkspace
		}
		if actor.Role != RoleOwner {
			return ErrForbidden
		}
		target, err := tx.WorkspaceForMember(ctx, workspaceID, accountID)
		if err != nil {
			return err
		}
		if target.Role == RoleOwner && role != RoleOwner {
			if err := requireAnotherOwner(ctx, tx, workspaceID); err != nil {
				return err
			}
		}
		return tx.UpdateMemberRole(ctx, workspaceID, accountID, role)
	})
}

// RemoveMember removes accountID from the organization. Anyone may remove
// themselves (leave); owners may remove anyone; admins may remove members.
func (s *Service) RemoveMember(ctx context.Context, actorID, workspaceID, accountID int64) error {
	return s.Store.InTx(ctx, func(tx *Store) error {
		actor, err := tx.WorkspaceForMember(ctx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if actor.Kind != KindOrganization {
			return ErrPersonalWorkspace
		}
		target, err := tx.WorkspaceForMember(ctx, workspaceID, accountID)
		if err != nil {
			return err
		}
		allowed := actorID == accountID ||
			actor.Role == RoleOwner ||
			(actor.Role == RoleAdmin && target.Role == RoleMember)
		if !allowed {
			return ErrForbidden
		}
		if target.Role == RoleOwner {
			if err := requireAnotherOwner(ctx, tx, workspaceID); err != nil {
				return err
			}
		}
		return tx.DeleteMembership(ctx, workspaceID, accountID)
	})
}

func requireAnotherOwner(ctx context.Context, tx *Store, workspaceID int64) error {
	owners, err := tx.CountOwners(ctx, workspaceID)
	if err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}

// --- invites ---

func (s *Service) PendingInvites(ctx context.Context, actorID, workspaceID int64) ([]Invite, error) {
	if _, err := s.organizationForManager(ctx, actorID, workspaceID); err != nil {
		return nil, err
	}
	return s.Store.PendingInvites(ctx, workspaceID, s.now())
}

// Invite whitelists email for the organization and emails them a one-time
// code and link. baseURL is the app's public origin, used to build the link.
// Inviting an email that already has a pending invite replaces that invite.
func (s *Service) Invite(ctx context.Context, actorID, workspaceID int64, email string, role Role, baseURL string) (CreatedInvite, error) {
	email, err := cleanEmail(email)
	if err != nil {
		return CreatedInvite{}, err
	}
	if role == "" {
		role = RoleMember
	}
	if role != RoleMember && role != RoleAdmin {
		return CreatedInvite{}, invalidInput("invites can be for members or admins")
	}
	ws, err := s.organizationForManager(ctx, actorID, workspaceID)
	if err != nil {
		return CreatedInvite{}, err
	}
	actor, err := s.Store.AccountByID(ctx, actorID)
	if err != nil {
		return CreatedInvite{}, err
	}
	code, err := newInviteCode()
	if err != nil {
		return CreatedInvite{}, err
	}
	now := s.now()
	var invite Invite
	err = s.Store.InTx(ctx, func(tx *Store) error {
		if member, err := tx.IsMemberByEmail(ctx, workspaceID, email); err != nil {
			return err
		} else if member {
			return ErrAlreadyMember
		}
		if err := tx.RevokePendingInvites(ctx, workspaceID, email, now); err != nil {
			return err
		}
		invite, err = tx.InsertInvite(ctx, workspaceID, email, role, hashInviteCode(code), actorID, now.Add(inviteLifetime))
		return err
	})
	if err != nil {
		return CreatedInvite{}, err
	}
	invite.InvitedBy = actor.Name

	created := CreatedInvite{Invite: invite, Code: code, Link: strings.TrimRight(baseURL, "/") + "/invite/" + code}
	if err := s.Mailer.Send(ctx, inviteEmail(email, actor.Name, ws.Name, code, created.Link)); err != nil {
		if !errors.Is(err, ErrMailerNotConfigured) {
			log.Printf("invite email to %s: %v", email, err)
		}
		created.EmailError = err.Error()
	} else {
		created.EmailSent = true
	}
	return created, nil
}

func (s *Service) RevokeInvite(ctx context.Context, actorID, workspaceID, inviteID int64) error {
	if _, err := s.organizationForManager(ctx, actorID, workspaceID); err != nil {
		return err
	}
	return s.Store.RevokeInvite(ctx, workspaceID, inviteID, s.now())
}

// PreviewInvite describes a pending invite to whoever holds its code, so the
// invite page can say who invited them where, before they sign in.
func (s *Service) PreviewInvite(ctx context.Context, code string) (InvitePreview, error) {
	inv, err := s.Store.PendingInviteByCodeHash(ctx, hashInviteCode(code), s.now())
	if errors.Is(err, ErrNotFound) {
		return InvitePreview{}, ErrInviteInvalid
	}
	if err != nil {
		return InvitePreview{}, err
	}
	return InvitePreview{WorkspaceName: inv.WorkspaceName, Email: inv.Email, InvitedBy: inv.InvitedBy}, nil
}

// AcceptInvite consumes the invite and adds the account to the organization.
// The account's email must be the one the invite was sent to.
func (s *Service) AcceptInvite(ctx context.Context, accountID int64, code string) (Workspace, error) {
	account, err := s.Store.AccountByID(ctx, accountID)
	if err != nil {
		return Workspace{}, err
	}
	var workspace Workspace
	err = s.Store.InTx(ctx, func(tx *Store) error {
		now := s.now()
		inv, err := tx.PendingInviteByCodeHash(ctx, hashInviteCode(code), now)
		if errors.Is(err, ErrNotFound) {
			return ErrInviteInvalid
		}
		if err != nil {
			return err
		}
		if !strings.EqualFold(inv.Email, account.Email) {
			return ErrInviteEmail
		}
		if _, err := tx.WorkspaceForMember(ctx, inv.WorkspaceID, accountID); err == nil {
			return ErrAlreadyMember
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := tx.MarkInviteAccepted(ctx, inv.ID, accountID, now); errors.Is(err, ErrNotFound) {
			return ErrInviteInvalid
		} else if err != nil {
			return err
		}
		if err := tx.InsertMembership(ctx, inv.WorkspaceID, accountID, inv.Role); err != nil {
			return err
		}
		if err := tx.SetUsage(ctx, accountID, UsageOrganization); err != nil {
			return err
		}
		// The code was emailed to this address, so holding it proves the
		// address too.
		if err := tx.MarkEmailVerified(ctx, accountID, now); err != nil {
			return err
		}
		workspace = Workspace{ID: inv.WorkspaceID, Kind: KindOrganization, Name: inv.WorkspaceName, Role: inv.Role}
		return nil
	})
	return workspace, err
}

func inviteEmail(to, inviter, workspace, code, link string) Email {
	subject := fmt.Sprintf("%s invited you to %s on Aycorn", inviter, workspace)
	text := fmt.Sprintf("%s invited you to join %s on Aycorn.\n\nAccept the invite: %s\n\nOr enter this code when you sign in: %s\n\nThe invite works once and expires in 7 days.",
		inviter, workspace, link, code)
	h := html.EscapeString
	body := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;font-size:14px;line-height:1.5;color:#171717">
<p><strong>%s</strong> invited you to join <strong>%s</strong> on Aycorn.</p>
<p><a href="%s" style="display:inline-block;padding:8px 14px;border-radius:6px;background:#171717;color:#fff;text-decoration:none">Accept invite</a></p>
<p>Or enter this code when you sign in:<br><code style="font-size:16px;letter-spacing:1px">%s</code></p>
<p style="color:#737373">The invite works once and expires in 7 days.</p>
</div>`, h(inviter), h(workspace), h(link), h(code))
	return Email{To: to, Subject: subject, Text: text, HTML: body}
}

func verificationEmail(to, name, link string) Email {
	text := fmt.Sprintf("Hi %s,\n\nConfirm your email address to start using Aycorn: %s\n\nThe link expires in 48 hours. If you didn't sign up, you can ignore this email.",
		name, link)
	h := html.EscapeString
	body := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;font-size:14px;line-height:1.5;color:#171717">
<p>Hi %s,</p>
<p>Confirm your email address to start using Aycorn.</p>
<p><a href="%s" style="display:inline-block;padding:8px 14px;border-radius:6px;background:#171717;color:#fff;text-decoration:none">Confirm email</a></p>
<p style="color:#737373">The link expires in 48 hours. If you didn't sign up, you can ignore this email.</p>
</div>`, h(name), h(link))
	return Email{To: to, Subject: "Confirm your email for Aycorn", Text: text, HTML: body}
}

func passwordResetEmail(to, name, link string) Email {
	text := fmt.Sprintf("Hi %s,\n\nReset your Aycorn password: %s\n\nThe link works once and expires in 1 hour. If you didn't ask for this, you can ignore this email; your password hasn't changed.",
		name, link)
	h := html.EscapeString
	body := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;font-size:14px;line-height:1.5;color:#171717">
<p>Hi %s,</p>
<p>Someone asked to reset your Aycorn password.</p>
<p><a href="%s" style="display:inline-block;padding:8px 14px;border-radius:6px;background:#171717;color:#fff;text-decoration:none">Choose a new password</a></p>
<p style="color:#737373">The link works once and expires in 1 hour. If you didn't ask for this, you can ignore this email; your password hasn't changed.</p>
</div>`, h(name), h(link))
	return Email{To: to, Subject: "Reset your Aycorn password", Text: text, HTML: body}
}

// --- input cleaning ---

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", invalidInput(fmt.Sprintf("name must be 1–%d characters", maxNameLength))
	}
	return name, nil
}

func cleanEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return "", invalidInput("enter a valid email address")
	}
	return email, nil
}

func cleanNameAndEmail(name, email string) (string, string, error) {
	name, err := cleanName(name)
	if err != nil {
		return "", "", err
	}
	email, err = cleanEmail(email)
	return name, email, err
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return invalidInput(fmt.Sprintf("password must be at least %d characters", minPasswordLength))
	}
	if len(password) > maxPasswordBytes {
		return invalidInput("password is too long")
	}
	return nil
}
