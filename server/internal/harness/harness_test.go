package harness

import (
	"context"
	"strings"
	"testing"
)

func TestReadOnlyShim_ReturnsMarkdown(t *testing.T) {
	shim := NewReadOnlyShim()
	spec := RunSpec{
		JobID:       42,
		TaskID:      7,
		PersonaID:   3,
		TaskName:    "Fix flaky test",
		TaskBody:    `[{"type":"p","children":[{"text":"details"}]}]`,
		AllowedTools: []string{"read", "search"},
	}
	res, err := shim.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d; want 0", res.ExitCode)
	}
	if !strings.Contains(res.Output, "# Research notes for task 7") {
		t.Fatalf("Output missing header: %q", res.Output)
	}
	if !strings.Contains(res.Output, "Fix flaky test") {
		t.Fatalf("Output missing task name: %q", res.Output)
	}
	if !strings.Contains(res.Output, "read via `search_tasks`") {
		t.Fatalf("Output missing expected shim blurb: %q", res.Output)
	}
	if res.Summary == "" {
		t.Fatal("Summary empty")
	}
	if res.UsageJson == "" {
		t.Fatal("UsageJson empty")
	}
}

func TestReadOnlyShim_RespectsContextCancel(t *testing.T) {
	shim := NewReadOnlyShim()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := shim.Run(ctx, RunSpec{TaskID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled ctx, got nil")
	}
}

func TestReadOnlyShim_EmptyTools(t *testing.T) {
	shim := NewReadOnlyShim()
	res, err := shim.Run(context.Background(), RunSpec{TaskID: 1, TaskName: "t", AllowedTools: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "(none") {
		t.Fatalf("expected none placeholder for empty tools, got %q", res.Output)
	}
}
