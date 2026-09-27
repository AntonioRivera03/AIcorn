package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
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
	// publicURL is the origin used in emailed links. When empty, links use
	// the origin the request came in on.
	publicURL string
	limits    *authLimits
	// trustProxy takes client addresses from X-Forwarded-For, for rate
	// limiting behind a reverse proxy.
	trustProxy bool
	// previewSignIn signs every visitor into one shared reviewer account, so
	// an application preview needs no signup (see preview.go).
	previewSignIn bool
	previewMu     sync.Mutex
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health/ready", s.readiness)
	mux.HandleFunc("GET /api/preview", getPreviewInfo)

	mux.HandleFunc("POST /api/auth/signup", s.limitByIP(s.signup))
	mux.HandleFunc("POST /api/auth/login", s.limitByIP(s.login))
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.withAccount(s.getMe))
	mux.HandleFunc("PUT /api/auth/me", s.withAccount(s.putMe))
	mux.HandleFunc("PUT /api/auth/me/usage", s.withAccount(s.putUsage))
	mux.HandleFunc("PUT /api/auth/me/password", s.withAccount(s.putPassword))
	mux.HandleFunc("POST /api/auth/verify-email", s.limitByIP(s.verifyEmail))
	mux.HandleFunc("POST /api/auth/verify-email/resend", s.withAccount(s.resendVerification))
	mux.HandleFunc("POST /api/auth/password-reset", s.limitByIP(s.requestPasswordReset))
	mux.HandleFunc("POST /api/auth/password-reset/confirm", s.limitByIP(s.resetPassword))

	// Everything that reaches workspace data or other people needs a
	// confirmed email address. Accepting an invite doesn't: it confirms it.
	mux.HandleFunc("POST /api/workspaces", s.withVerifiedAccount(s.postOrganization))
	mux.HandleFunc("PUT /api/workspaces/{workspaceId}", s.withVerifiedAccount(s.putWorkspace))
	mux.HandleFunc("GET /api/workspaces/{workspaceId}/members", s.withVerifiedAccount(s.getMembers))
	mux.HandleFunc("PUT /api/workspaces/{workspaceId}/members/{accountId}", s.withVerifiedAccount(s.putMember))
	mux.HandleFunc("DELETE /api/workspaces/{workspaceId}/members/{accountId}", s.withVerifiedAccount(s.deleteMember))
	mux.HandleFunc("GET /api/workspaces/{workspaceId}/invites", s.withVerifiedAccount(s.getInvites))
	mux.HandleFunc("POST /api/workspaces/{workspaceId}/invites", s.withVerifiedAccount(s.postInvite))
	mux.HandleFunc("DELETE /api/workspaces/{workspaceId}/invites/{inviteId}", s.withVerifiedAccount(s.deleteInvite))

	mux.HandleFunc("GET /api/invites/{code}", s.limitByIP(s.getInvite))
	mux.HandleFunc("POST /api/invites/accept", s.limitByIP(s.withAccount(s.acceptInvite)))

	// Everything else under /api/ belongs to a workspace. Registered per method
	// so these patterns stay more specific than the SPA's "GET /".
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /api/", s.withVerifiedAccount(s.forwardToWorkspace))
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
		account, renewed, err := s.accounts.Authenticate(r.Context(), sessionToken(r))
		if errors.Is(err, accounts.ErrUnauthenticated) && s.previewSignIn {
			var session accounts.Session
			account, session, err = s.previewSession(r.Context())
			renewed = &session
		}
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

// withVerifiedAccount is withAccount for requests that need a confirmed
// email address.
func (s *server) withVerifiedAccount(next accountHandler) http.HandlerFunc {
	return s.withAccount(func(w http.ResponseWriter, r *http.Request, account accounts.Account) {
		if !account.EmailVerified {
			http.Error(w, accounts.ErrEmailUnverified.Error(), http.StatusForbidden)
			return
		}
		next(w, r, account)
	})
}

// sessionToken is the session cookie's value, or "" without one.
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
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

// baseURL is the origin emailed links (invites, confirmations, password
// resets) point at.
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
		errors.Is(err, accounts.ErrLastOwner),
		errors.Is(err, accounts.ErrTokenInvalid):
		return http.StatusBadRequest, true
	case errors.Is(err, accounts.ErrInvalidCredentials),
		errors.Is(err, accounts.ErrUnauthenticated):
		return http.StatusUnauthorized, true
	case errors.Is(err, accounts.ErrForbidden),
		errors.Is(err, accounts.ErrInviteEmail),
		errors.Is(err, accounts.ErrEmailUnverified):
		return http.StatusForbidden, true
	case errors.Is(err, accounts.ErrNotFound),
		errors.Is(err, accounts.ErrInviteInvalid):
		return http.StatusNotFound, true
	case errors.Is(err, accounts.ErrEmailTaken),
		errors.Is(err, accounts.ErrAlreadyMember):
		return http.StatusConflict, true
	case errors.Is(err, accounts.ErrMailerNotConfigured):
		return http.StatusServiceUnavailable, true
	}
	return 0, false
}
