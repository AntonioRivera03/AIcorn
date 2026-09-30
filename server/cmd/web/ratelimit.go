package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimiter counts events per key in fixed windows. It lives in memory, so
// counts reset when the server restarts; that's enough to make password
// guessing and email flooding impractical against a single server.
type rateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	windows   map[string]*rateWindow
	lastSweep time.Time
}

type rateWindow struct {
	count int
	ends  time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, now: time.Now, windows: map[string]*rateWindow{}}
}

// current returns key's live window, starting a new one if the last ended.
// Callers hold mu.
func (l *rateLimiter) current(key string, now time.Time) *rateWindow {
	if now.Sub(l.lastSweep) > l.window {
		for k, w := range l.windows {
			if !now.Before(w.ends) {
				delete(l.windows, k)
			}
		}
		l.lastSweep = now
	}
	w, ok := l.windows[key]
	if !ok || !now.Before(w.ends) {
		w = &rateWindow{ends: now.Add(l.window)}
		l.windows[key] = w
	}
	return w
}

// allow records an event for key. When key is over its limit it returns
// false and how long until the window resets.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.current(key, now)
	if w.count >= l.limit {
		return false, w.ends.Sub(now)
	}
	w.count++
	return true, 0
}

// blocked reports, without recording anything, whether key is over its limit.
func (l *rateLimiter) blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.current(key, now)
	if w.count >= l.limit {
		return true, w.ends.Sub(now)
	}
	return false, 0
}

// add records an event for key without checking the limit.
func (l *rateLimiter) add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.current(key, l.now()).count++
}

func (l *rateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, key)
}

// authLimits are the limits on the account endpoints anyone can reach.
type authLimits struct {
	// perIP caps every sign-in, sign-up, password reset, link, and invite
	// lookup from one address.
	perIP *rateLimiter
	// failedLogins caps wrong passwords per account, however many addresses
	// they come from. A successful login clears it.
	failedLogins *rateLimiter
	// emails caps how often one address or account can be sent a reset or
	// confirmation email, so the forms can't be used to flood an inbox.
	emails *rateLimiter
}

func newAuthLimits() *authLimits {
	return &authLimits{
		perIP:        newRateLimiter(60, 15*time.Minute),
		failedLogins: newRateLimiter(10, 15*time.Minute),
		emails:       newRateLimiter(5, time.Hour),
	}
}

// limitByIP rejects the request when its address is over the perIP limit.
func (s *server) limitByIP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ok, retry := s.limits.perIP.allow("ip:" + s.clientIP(r)); !ok {
			tooManyRequests(w, retry)
			return
		}
		next(w, r)
	}
}

// clientIP is the address a request came from. Behind a reverse proxy every
// request arrives from the proxy, so with trustProxy set it's the last
// X-Forwarded-For entry instead: the one the proxy itself appended. (Earlier
// entries come from the client and can't be trusted.)
func (s *server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
			hops := strings.Split(forwarded[len(forwarded)-1], ",")
			if ip := strings.TrimSpace(hops[len(hops)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func tooManyRequests(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
	minutes := int(math.Ceil(retry.Minutes()))
	unit := "minutes"
	if minutes == 1 {
		unit = "minute"
	}
	http.Error(w, fmt.Sprintf("too many attempts; try again in %d %s", minutes, unit), http.StatusTooManyRequests)
}
