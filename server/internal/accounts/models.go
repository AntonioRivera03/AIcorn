package accounts

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrEmailTaken         = errors.New("an account with this email already exists")
	ErrInvalidCredentials = errors.New("incorrect email or password")
	ErrUnauthenticated    = errors.New("not signed in")
	ErrForbidden          = errors.New("you don't have permission to do that")
	ErrNotFound           = errors.New("not found")
	ErrInviteInvalid      = errors.New("this invite is invalid, expired, or already used")
	ErrInviteEmail        = errors.New("this invite was sent to a different email address")
	ErrAlreadyMember      = errors.New("already a member of this organization")
	ErrLastOwner          = errors.New("an organization needs at least one owner")
	ErrPersonalWorkspace  = errors.New("personal workspaces can't have other members")
)

// invalidInputError carries a message fit to show the user as-is, while still
// matching errors.Is(err, ErrInvalidInput) for status mapping.
type invalidInputError struct{ message string }

func (e invalidInputError) Error() string        { return e.message }
func (e invalidInputError) Is(target error) bool { return target == ErrInvalidInput }

func invalidInput(message string) error { return invalidInputError{message} }

type Usage string

const (
	UsageSolo         Usage = "solo"
	UsageOrganization Usage = "organization"
)

type WorkspaceKind string

const (
	KindPersonal     WorkspaceKind = "personal"
	KindOrganization WorkspaceKind = "organization"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// canManageMembers reports whether a role may invite people and remove
// ordinary members.
func (r Role) canManageMembers() bool { return r == RoleOwner || r == RoleAdmin }

func validRole(r Role) bool { return r == RoleOwner || r == RoleAdmin || r == RoleMember }

type Account struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	// Usage is empty until the account finishes onboarding.
	Usage Usage `json:"usage"`
}

// Workspace is a workspace as seen by one of its members.
type Workspace struct {
	ID   int64         `json:"id"`
	Kind WorkspaceKind `json:"kind"`
	Name string        `json:"name"`
	Role Role          `json:"role"`
}

type Member struct {
	AccountID  int64      `json:"accountId"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Role       Role       `json:"role"`
	TimeJoined *time.Time `json:"timeJoined"`
}

type Invite struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Role        Role       `json:"role"`
	InvitedBy   string     `json:"invitedBy"`
	TimeCreated *time.Time `json:"timeCreated"`
	TimeExpires time.Time  `json:"timeExpires"`
}

// CreatedInvite is returned once, to the inviter, right after creation. It's
// the only time the plain code exists outside the invitee's email.
type CreatedInvite struct {
	Invite
	Code      string `json:"code"`
	Link      string `json:"link"`
	EmailSent bool   `json:"emailSent"`
	// EmailError explains why the email didn't go out (e.g. no Resend key),
	// so the inviter knows to share the link themselves.
	EmailError string `json:"emailError,omitempty"`
}

// InvitePreview is what anyone holding an invite code may see about it.
type InvitePreview struct {
	WorkspaceName string `json:"workspaceName"`
	Email         string `json:"email"`
	InvitedBy     string `json:"invitedBy"`
}
