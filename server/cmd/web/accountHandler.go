package main

import (
	"net/http"

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
	}
	if !decodeJSONInput(w, r, &in) {
		return
	}
	account, session, err := s.accounts.Signup(r.Context(), in.Name, in.Email, in.Password)
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
	account, session, err := s.accounts.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		respondErr(w, err)
		return
	}
	setSessionCookie(w, r, session)
	s.writeMe(w, r, http.StatusOK, account)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.accounts.Logout(r.Context(), c.Value); err != nil {
			respondErr(w, err)
			return
		}
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
