package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/accounts"
)

const (
	sessionCookie = "aycorn_session"
	// workspaceHeader carries the workspace a request is for. The frontend sets
	// it on every API call from the workspace open in that tab, so two tabs on
	// different workspaces never write into each other's data.
	workspaceHeader = "X-Aycorn-Workspace"
)

// server is the multi-user front door. It serves the public pages' API
// (signup, login, invites), the account and organization API, and forwards
// every other /api/ request to the requested workspace's app after checking
// the caller belongs to it.
type server struct {
	accounts   *accounts.Service
	workspaces *workspaceRegistry
	accountsDB *sql.DB
	// publicURL is the origin used in invite links. When empty, links use the
	// origin the inviter's request came in on.
	publicURL string
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health/ready", s.readiness)
	mux.HandleFunc("GET /api/preview", getPreviewInfo)

	mux.HandleFunc("POST /api/auth/signup", s.signup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.withAccount(s.getMe))
	mux.HandleFunc("PUT /api/auth/me", s.withAccount(s.putMe))
	mux.HandleFunc("PUT /api/auth/me/usage", s.withAccount(s.putUsage))

	mux.HandleFunc("POST /api/workspaces", s.withAccount(s.postOrganization))
	mux.HandleFunc("PUT /api/workspaces/{workspaceId}", s.withAccount(s.putWorkspace))
	mux.HandleFunc("GET /api/workspaces/{workspaceId}/members", s.withAccount(s.getMembers))
	mux.HandleFunc("PUT /api/workspaces/{workspaceId}/members/{accountId}", s.withAccount(s.putMember))
	mux.HandleFunc("DELETE /api/workspaces/{workspaceId}/members/{accountId}", s.withAccount(s.deleteMember))
	mux.HandleFunc("GET /api/workspaces/{workspaceId}/invites", s.withAccount(s.getInvites))
	mux.HandleFunc("POST /api/workspaces/{workspaceId}/invites", s.withAccount(s.postInvite))
	mux.HandleFunc("DELETE /api/workspaces/{workspaceId}/invites/{inviteId}", s.withAccount(s.deleteInvite))

	mux.HandleFunc("GET /api/invites/{code}", s.getInvite)
	mux.HandleFunc("POST /api/invites/accept", s.withAccount(s.acceptInvite))

	// Everything else under /api/ belongs to a workspace. Registered per method
	// so these patterns stay more specific than the SPA's "GET /".
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /api/", s.withAccount(s.forwardToWorkspace))
	}

	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, If-Match, "+workspaceHeader)
		w.WriteHeader(http.StatusNoContent)
	})
	if spa := spaHandler(); spa != nil {
		mux.Handle("GET /", spa)
	}
	return withCommon(mux)
}

// --- authentication ---

type accountKey struct{}

// accountFromContext returns the signed-in account of an authenticated
// request, including requests forwarded to a workspace.
func accountFromContext(ctx context.Context) (accounts.Account, bool) {
	account, ok := ctx.Value(accountKey{}).(accounts.Account)
	return account, ok
}

type accountHandler func(w http.ResponseWriter, r *http.Request, account accounts.Account)

// withAccount rejects requests without a live session and passes the
// signed-in account on.
func (s *server) withAccount(next accountHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var token string
		if c, err := r.Cookie(sessionCookie); err == nil {
			token = c.Value
		}
		account, renewed, err := s.accounts.Authenticate(r.Context(), token)
		if errors.Is(err, accounts.ErrUnauthenticated) {
			// The normal state of a signed-out visitor; not worth a log line.
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if err != nil {
			respondErr(w, err)
			return
		}
		if renewed != nil {
			setSessionCookie(w, r, *renewed)
		}
		next(w, r.WithContext(context.WithValue(r.Context(), accountKey{}, account)), account)
	}
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, session accounts.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.Expires,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// --- workspace forwarding ---

func (s *server) forwardToWorkspace(w http.ResponseWriter, r *http.Request, account accounts.Account) {
	id, err := strconv.ParseInt(r.Header.Get(workspaceHeader), 10, 64)
	if err != nil {
		http.Error(w, "missing or invalid "+workspaceHeader+" header", http.StatusBadRequest)
		return
	}
	if _, err := s.accounts.WorkspaceForMember(r.Context(), id, account.ID); err != nil {
		respondErr(w, err)
		return
	}
	rt, err := s.workspaces.get(id)
	if err != nil {
		log.Println(err)
		http.Error(w, "this workspace is unavailable right now", http.StatusServiceUnavailable)
		return
	}
	rt.handler.ServeHTTP(w, r)
}

// --- helpers ---

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// baseURL is the origin invite links point at.
func (s *server) baseURL(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func accountsStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, accounts.ErrInvalidInput),
		errors.Is(err, accounts.ErrPersonalWorkspace),
		errors.Is(err, accounts.ErrLastOwner):
		return http.StatusBadRequest, true
	case errors.Is(err, accounts.ErrInvalidCredentials),
		errors.Is(err, accounts.ErrUnauthenticated):
		return http.StatusUnauthorized, true
	case errors.Is(err, accounts.ErrForbidden),
		errors.Is(err, accounts.ErrInviteEmail):
		return http.StatusForbidden, true
	case errors.Is(err, accounts.ErrNotFound),
		errors.Is(err, accounts.ErrInviteInvalid):
		return http.StatusNotFound, true
	case errors.Is(err, accounts.ErrEmailTaken),
		errors.Is(err, accounts.ErrAlreadyMember):
		return http.StatusConflict, true
	}
	return 0, false
}
