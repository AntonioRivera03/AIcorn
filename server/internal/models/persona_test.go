package models_test

import (
	"reflect"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

func TestPersonaHarnessIsValid_whenValueIsCurated(t *testing.T) {
	for _, harness := range models.PersonaHarnesses {
		if !models.IsValidPersonaHarness(harness) {
			t.Fatalf("expected curated harness %q to be valid", harness)
		}
	}
}

func TestPersonaHarnessIsValid_whenValueIsUnknown(t *testing.T) {
	if models.IsValidPersonaHarness("unknown") {
		t.Fatal("expected unknown harness to be invalid")
	}
}

func TestPersonaModelIsValid_whenValueIsCurated(t *testing.T) {
	for _, model := range models.PersonaModels {
		if !models.IsValidPersonaModel(model) {
			t.Fatalf("expected curated model %q to be valid", model)
		}
	}
}

func TestPersonaModelIsValid_whenValueIsUnknown(t *testing.T) {
	if models.IsValidPersonaModel("unknown") {
		t.Fatal("expected unknown model to be invalid")
	}
}

func TestParseAllowedTools_whenValueIsOmittedNullOrEmpty(t *testing.T) {
	for _, raw := range []string{"", " ", "null", " null ", "[]"} {
		tools, err := models.ParseAllowedTools(raw)
		if err != nil {
			t.Fatalf("ParseAllowedTools(%q) returned error: %v", raw, err)
		}
		if tools == nil {
			t.Fatalf("ParseAllowedTools(%q) returned nil; expected restrictive empty list", raw)
		}
		if len(tools) != 0 {
			t.Fatalf("ParseAllowedTools(%q) = %v; expected no tools", raw, tools)
		}
	}
}

func TestParseAllowedTools_whenValueIsJSONArray(t *testing.T) {
	tools, err := models.ParseAllowedTools(`["read_task","search_tasks"]`)
	if err != nil {
		t.Fatalf("ParseAllowedTools returned error: %v", err)
	}

	want := []string{"read_task", "search_tasks"}
	if !reflect.DeepEqual(tools, want) {
		t.Fatalf("ParseAllowedTools = %v; want %v", tools, want)
	}
}

func TestParseAllowedTools_whenValueIsNotJSONArray(t *testing.T) {
	if _, err := models.ParseAllowedTools(`{"read_task":true}`); err == nil {
		t.Fatal("expected non-array JSON to be rejected")
	}
}

func TestEncodeAllowedTools_whenValueIsNil(t *testing.T) {
	encoded, err := models.EncodeAllowedTools(nil)
	if err != nil {
		t.Fatalf("EncodeAllowedTools returned error: %v", err)
	}
	if encoded != "[]" {
		t.Fatalf("EncodeAllowedTools(nil) = %q; want []", encoded)
	}
}

func TestNormalizeBody_whenBodyIsEmptyOrNotPlateJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "whitespace", body: "   "},
		{name: "empty array", body: "[]"},
		{name: "null", body: "null"},
		{name: "encoded empty string", body: `""`},
		{name: "legacy plain text", body: "legacy_token"},
		{name: "JSON object", body: `{"type":"p"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an empty-compatible or non-Plate body representation
			// When it crosses the model normalization boundary
			got := models.NormalizeBody(test.body)

			// Then it becomes the canonical empty Plate document
			if got != models.EmptyBody {
				t.Fatalf("NormalizeBody(%q) = %q; want %q", test.body, got, models.EmptyBody)
			}
		})
	}
}

func TestNormalizeBody_whenBodyIsPlateJSONArray(t *testing.T) {
	// Given a non-empty Plate document with surrounding storage whitespace
	body := `  [{"type":"p","children":[{"text":"structured_token"}]}]  `

	// When it crosses the model normalization boundary
	got := models.NormalizeBody(body)

	// Then the original serialized document is preserved byte-for-byte
	if got != body {
		t.Fatalf("NormalizeBody() = %q; want original %q", got, body)
	}
}
