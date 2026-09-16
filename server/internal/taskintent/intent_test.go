package taskintent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestJevIntentRouting(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"question", `{"answers":{"intent":{"type":"choice","choice":"question","confidence":0.96,"probabilities":{"question":0.98,"work":0.01,"uncertain":0.01}}}}`, "question"},
		{"work", `{"answers":{"intent":{"type":"choice","choice":"work","confidence":0.96,"probabilities":{"question":0.01,"work":0.98,"uncertain":0.01}}}}`, "work"},
		{"ambiguous", `{"answers":{"intent":{"type":"choice","choice":"question","confidence":0.1,"probabilities":{"question":0.51,"work":0.48,"uncertain":0.01}}}}`, "uncertain"},
		{"unknown", `{"answers":{"intent":{"type":"choice","choice":"execute","confidence":1,"probabilities":{"execute":1}}}}`, "uncertain"},
		{"invalid probabilities", `{"answers":{"intent":{"type":"choice","choice":"question","confidence":1,"probabilities":{"question":1,"work":1,"uncertain":1}}}}`, "uncertain"},
		{"missing", `{}`, "uncertain"}, {"malformed", `broken`, "uncertain"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			j := Jev{Key: "test-key", Client: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.typesafe.ai/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatal("wrong endpoint or authentication")
				}
				var body struct {
					Model     string                     `json:"model"`
					State     Context                    `json:"state"`
					Questions map[string]json.RawMessage `json:"questions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Model != "jev-latest" || body.Questions["intent"] == nil || len(body.State.LastAnswer) > 8000 {
					t.Fatal("wrong bounded context")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}}
			got := j.Classify(context.Background(), Context{Message: "Can you fix this?", LastAnswer: strings.Repeat("a", 20000)})
			if got.Intent != test.want {
				t.Fatal(got)
			}
		})
	}
}
func TestJevUnavailableNeverSilentlyStartsWork(t *testing.T) {
	if got := (Jev{}).Classify(context.Background(), Context{}); got.Intent != "uncertain" || got.Provider != "manual" {
		t.Fatal(got)
	}
	j := Jev{Key: "test-key", Client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("private provider error"))}, nil
	})}}
	if got := j.Classify(context.Background(), Context{}); got.Intent != "uncertain" || strings.Contains(got.Reason, "private") {
		t.Fatal(got)
	}
}
