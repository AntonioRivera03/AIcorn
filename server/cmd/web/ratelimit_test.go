package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterWindows(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newRateLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	for i := range 2 {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("event %d refused under the limit", i+1)
		}
	}
	ok, retry := l.allow("a")
	if ok || retry != time.Minute {
		t.Fatalf("third event: allowed=%v retry=%v", ok, retry)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("keys should be counted separately")
	}

	now = now.Add(time.Minute)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("a new window should start over")
	}

	l.add("c")
	l.add("c")
	if blocked, _ := l.blocked("c"); !blocked {
		t.Fatal("add should count toward the limit")
	}
	l.reset("c")
	if blocked, _ := l.blocked("c"); blocked {
		t.Fatal("reset should clear the key")
	}
}

func TestClientIPTrustsProxyOnlyWhenTold(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Add("X-Forwarded-For", "6.6.6.6, 203.0.113.9")

	if ip := (&server{}).clientIP(r); ip != "127.0.0.1" {
		t.Fatalf("without trustProxy the header must be ignored, got %s", ip)
	}
	// The client can prepend whatever it likes; only the entry the proxy
	// appended counts.
	if ip := (&server{trustProxy: true}).clientIP(r); ip != "203.0.113.9" {
		t.Fatalf("behind a proxy, got %s", ip)
	}
}
