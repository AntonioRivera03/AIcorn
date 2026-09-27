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
	maxNameLength      = 100
)

// Service holds the account and workspace rules. Provision is called inside
// the transaction that creates a workspace, so a workspace row never exists
// without its database.
type Service struct {
	Store     *Store
	Mailer    Mailer
	Provision func(ctx context.Context, workspaceID int64) error
	Now       func() time.Time
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

func (s *Service) Signup(ctx context.Context, name, email, password string) (Account, Session, error) {
	name, email, err := cleanNameAndEmail(name, email)
	if err != nil {
		return Account{}, Session{}, err
	}
	if err := validatePassword(password); err != nil {
		return Account{}, Session{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Account{}, Session{}, err
	}
	var account Account
	err = s.Store.InTx(ctx, func(tx *Store) error {
		if taken, err := tx.EmailExists(ctx, email); err != nil {
			return err
		} else if taken {
			return ErrEmailTaken
		}
		id, err := tx.InsertAccount(ctx, email, name, hash)
		if err != nil {
			return err
		}
		account = Account{ID: id, Email: email, Name: name}
		_, err = s.createWorkspace(ctx, tx, KindPersonal, "Personal", id)
		return err
	})
	if err != nil {
		return Account{}, Session{}, err
	}
	session, err := s.startSession(ctx, account.ID)
	return account, session, err
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
	token, err := newSessionToken()
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

// PruneSessions deletes expired sessions; run it periodically.
func (s *Service) PruneSessions(ctx context.Context) error {
	return s.Store.DeleteExpiredSessions(ctx, s.now())
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
	err = s.Store.InTx(ctx, func(tx *Store) error {
		workspace, err = s.createWorkspace(ctx, tx, KindOrganization, name, accountID)
		if err != nil {
			return err
		}
		return tx.SetUsage(ctx, accountID, UsageOrganization)
	})
	return workspace, err
}

// createWorkspace makes the workspace, its owner membership, and its
// database. For a personal workspace, owner is also the workspace's account.
func (s *Service) createWorkspace(ctx context.Context, tx *Store, kind WorkspaceKind, name string, owner int64) (Workspace, error) {
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
	if s.Provision != nil {
		if err := s.Provision(ctx, id); err != nil {
			return Workspace{}, fmt.Errorf("provisioning workspace %d: %w", id, err)
		}
	}
	return Workspace{ID: id, Kind: kind, Name: name, Role: RoleOwner}, nil
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
