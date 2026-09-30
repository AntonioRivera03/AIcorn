package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/waseem-polus/aycorn/server/internal/accounts"
)

type meResponse struct {
	Account    accounts.Account     `json:"account"`
	Workspaces []accounts.Workspace `json:"workspaces"`
}

func (s *server) writeMe(w http.ResponseWriter, r *http.Request, status int, account accounts.Account) {
	workspaces, err := s.accounts.Workspaces(r.Context(), account.ID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, status, meResponse{Account: account, Workspaces: workspaces})
}

// --- auth ---

func (s *server) signup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		// Invite is the code of the invite the person signed up from, if any.
		Invite string `json:"invite"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	account, session, err := s.accounts.Signup(r.Context(), accounts.SignupInput{
		Name: in.Name, Email: in.Email, Password: in.Password, InviteCode: in.Invite,
	}, s.baseURL(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	setSessionCookie(w, r, session)
	s.writeMe(w, r, http.StatusCreated, account)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	// Wrong passwords are counted per account as well as per address, so
	// guessing one account's password from many addresses stalls too.
	accountKey := strings.ToLower(strings.TrimSpace(in.Email))
	if blocked, retry := s.limits.failedLogins.blocked(accountKey); blocked {
		tooManyRequests(w, retry)
		return
	}
	account, session, err := s.accounts.Login(r.Context(), in.Email, in.Password)
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		s.limits.failedLogins.add(accountKey)
	}
	if err != nil {
		respondErr(w, err)
		return
	}
	s.limits.failedLogins.reset(accountKey)
	setSessionCookie(w, r, session)
	s.writeMe(w, r, http.StatusOK, account)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.accounts.Logout(r.Context(), sessionToken(r)); err != nil {
		respondErr(w, err)
		return
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getMe(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	s.writeMe(w, r, http.StatusOK, account)
}

func (s *server) putMe(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	account, err := s.accounts.Rename(r.Context(), account.ID, in.Name)
	if err != nil {
		respondErr(w, err)
		return
	}
	s.writeMe(w, r, http.StatusOK, account)
}

func (s *server) putUsage(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	var in struct {
		Usage accounts.Usage `json:"usage"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if err := s.accounts.SetUsage(r.Context(), account.ID, in.Usage); err != nil {
		respondErr(w, err)
		return
	}
	account.Usage = in.Usage
	s.writeMe(w, r, http.StatusOK, account)
}

func (s *server) putPassword(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if err := s.accounts.ChangePassword(r.Context(), account.ID, sessionToken(r), in.Current, in.New); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- email confirmation and password resets ---

// verifyEmail follows a confirmation link. It needs no session: the link can
// be opened in any browser.
func (s *server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if _, err := s.accounts.VerifyEmail(r.Context(), in.Token); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) resendVerification(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	if ok, retry := s.limits.emails.allow("verify:" + account.Email); !ok {
		tooManyRequests(w, retry)
		return
	}
	if err := s.accounts.SendVerification(r.Context(), account.ID, s.baseURL(r)); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requestPasswordReset answers the same whether or not the email has an
// account; emailSent is false only when this server can't send email at all.
func (s *server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if ok, retry := s.limits.emails.allow("reset:" + strings.ToLower(strings.TrimSpace(in.Email))); !ok {
		tooManyRequests(w, retry)
		return
	}
	emailSent, err := s.accounts.RequestPasswordReset(r.Context(), in.Email, s.baseURL(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"emailSent": emailSent})
}

// resetPassword sets a new password from a reset link and signs in.
func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	account, session, err := s.accounts.ResetPassword(r.Context(), in.Token, in.Password)
	if err != nil {
		respondErr(w, err)
		return
	}
	s.limits.failedLogins.reset(strings.ToLower(account.Email))
	setSessionCookie(w, r, session)
	s.writeMe(w, r, http.StatusOK, account)
}

// --- organizations ---

func (s *server) postOrganization(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	workspace, err := s.accounts.CreateOrganization(r.Context(), account.ID, in.Name)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (s *server) putWorkspace(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if err := s.accounts.RenameWorkspace(r.Context(), account.ID, id, in.Name); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteWorkspace(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	if err := s.accounts.DeleteOrganization(r.Context(), account.ID, id); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- members ---

func (s *server) getMembers(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	members, err := s.accounts.Members(r.Context(), account.ID, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *server) putMember(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	workspaceID, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	accountID, ok := pathID(w, r, "accountId")
	if !ok {
		return
	}
	var in struct {
		Role accounts.Role `json:"role"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	if err := s.accounts.SetMemberRole(r.Context(), account.ID, workspaceID, accountID, in.Role); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteMember(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	workspaceID, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	accountID, ok := pathID(w, r, "accountId")
	if !ok {
		return
	}
	if err := s.accounts.RemoveMember(r.Context(), account.ID, workspaceID, accountID); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- invites ---

func (s *server) getInvites(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	invites, err := s.accounts.PendingInvites(r.Context(), account.ID, id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

func (s *server) postInvite(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	var in struct {
		Email string        `json:"email"`
		Role  accounts.Role `json:"role"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	invite, err := s.accounts.Invite(r.Context(), account.ID, id, in.Email, in.Role, s.baseURL(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (s *server) deleteInvite(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	workspaceID, ok := pathID(w, r, "workspaceId")
	if !ok {
		return
	}
	inviteID, ok := pathID(w, r, "inviteId")
	if !ok {
		return
	}
	if err := s.accounts.RevokeInvite(r.Context(), account.ID, workspaceID, inviteID); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getInvite is public: the invite page shows who invited you where before
// you've signed in. Holding the code is the authorization.
func (s *server) getInvite(w http.ResponseWriter, r *http.Request) {
	preview, err := s.accounts.PreviewInvite(r.Context(), r.PathValue("code"))
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *server) acceptInvite(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	var in struct {
		Code string `json:"code"`
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	workspace, err := s.accounts.AcceptInvite(r.Context(), account.ID, in.Code)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}
