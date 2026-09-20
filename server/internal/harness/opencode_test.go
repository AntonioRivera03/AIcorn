package harness

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCodeTransportAndDisabledGate(t *testing.T) {
	c, err := newOpenCodeClient("http://127.0.0.1:4096", "/repo with spaces", "password")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("directory") != "/repo with spaces" {
			t.Fatal("lost directory scope", r.URL)
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "opencode" || password != "password" {
			t.Fatal("lost authentication")
		}
		body := `true`
		switch r.URL.Path {
		case "/session":
			body = `{"id":"session1"}`
		case "/session/session1/message":
			if r.Method == "POST" {
				var data map[string]any
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					t.Fatal(err)
				}
				if data["system"] == "" || data["model"].(map[string]any)["providerID"] != "openai" {
					t.Fatal(data)
				}
				body = `{"info":{"id":"message1","role":"assistant","tokens":{"input":5}},"parts":[{"type":"text","text":"answer"}]}`
			} else {
				body = `[]`
			}
		case "/event":
			body = "event: message\ndata: {\"type\":\"session.idle\",\n" + "data: \"properties\":{\"sessionID\":\"session1\"}}\n\n"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	id, err := c.startSession(context.Background(), "Ticket")
	if err != nil || id != "session1" {
		t.Fatal(id, err)
	}
	req := testRequest("")
	req.Model = "openai/gpt-5.6-sol"
	message, err := c.sendTurn(context.Background(), id, RunSpec{Request: req})
	if err != nil || message.Info.ID != "message1" || message.Parts[0].Text != "answer" {
		t.Fatal(message, err)
	}
	if _, err = c.readMessages(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err = c.abort(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	count := 0
	err = c.events(context.Background(), func(raw json.RawMessage) error {
		count++
		if !strings.Contains(string(raw), "session1") {
			t.Fatal(string(raw))
		}
		return nil
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) || count != 1 {
		t.Fatal(count, err)
	}
	if len(paths) != 5 {
		t.Fatal(paths)
	}
	if _, err = (&OpenCode{}).Run(context.Background(), RunSpec{Request: req}); err == nil {
		t.Fatal("disabled adapter ran")
	}
}

func TestOpenCodeRejectsRemoteAndReportsFailure(t *testing.T) {
	for _, address := range []string{"https://example.com", "http://10.0.0.1:4096", "http://user:password@127.0.0.1:4096", "http://localhost:4096"} {
		if _, err := newOpenCodeClient(address, "", ""); err == nil {
			t.Fatal(address)
		}
	}
	c, _ := newOpenCodeClient("http://127.0.0.1:4096", "", "")
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("secret"))}, nil
	})
	if _, err := c.startSession(context.Background(), "Ticket"); err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
