package markdown

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waseem-polus/aycorn/server/assets/bin"
)

// The converter is a subprocess boundary, so these tests drive the real bundled
// md-convert.cjs under a real node. Mocking the boundary away would only test
// the mock. The cost is a `node` dependency, which is checked up front.
func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("skipping: node is not on PATH (required by internal/markdown)")
	}
	// assets/bin/md-convert.cjs is gitignored, so a missing bundle normally
	// fails the //go:embed at compile time. An empty placeholder file gets past
	// that, though, and would fail here in a confusing way.
	if len(bin.MdConvert) == 0 {
		t.Skip("skipping: md-convert.cjs bundle is empty — run `make build-md-convert`")
	}
}

const sampleMarkdown = "# Title\n\nSome **bold** text.\n\n* a\n* b\n"

func TestToBodyProducesPlateJSON(t *testing.T) {
	requireNode(t)

	bodies, err := (&Converter{}).ToBody(context.Background(), []string{sampleMarkdown})
	if err != nil {
		t.Fatalf("ToBody: %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("got %d bodies, want 1", len(bodies))
	}

	var nodes []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &nodes); err != nil {
		t.Fatalf("body is not a Plate document array: %v (%s)", err, bodies[0])
	}
	if len(nodes) == 0 {
		t.Fatal("body has no nodes")
	}
	if nodes[0].Type != "h1" {
		t.Errorf("first node type = %q, want %q", nodes[0].Type, "h1")
	}
}

func TestRoundTripIsStable(t *testing.T) {
	requireNode(t)

	converter := &Converter{}
	ctx := context.Background()

	once, err := roundTrip(ctx, converter, sampleMarkdown)
	if err != nil {
		t.Fatalf("first round trip: %v", err)
	}
	twice, err := roundTrip(ctx, converter, once)
	if err != nil {
		t.Fatalf("second round trip: %v", err)
	}

	// The serializer's own output is the canonical form, so converting it again
	// must change nothing. A body that drifted on every MCP read/write cycle
	// would rewrite the user's task on each edit.
	if once != twice {
		t.Errorf("round trip is not stable:\nfirst:  %q\nsecond: %q", once, twice)
	}
	for _, want := range []string{"# Title", "**bold**", "* a"} {
		if !strings.Contains(once, want) {
			t.Errorf("round-tripped markdown %q is missing %q", once, want)
		}
	}
}

func roundTrip(ctx context.Context, c *Converter, markdown string) (string, error) {
	bodies, err := c.ToBody(ctx, []string{markdown})
	if err != nil {
		return "", err
	}
	markdowns, err := c.ToMarkdown(ctx, bodies)
	if err != nil {
		return "", err
	}
	return markdowns[0], nil
}

func TestBatchConversionIsPositional(t *testing.T) {
	requireNode(t)

	bodies, err := (&Converter{}).ToBody(context.Background(), []string{"# One", "# Two", "# Three"})
	if err != nil {
		t.Fatalf("ToBody: %v", err)
	}

	markdowns, err := (&Converter{}).ToMarkdown(context.Background(), bodies)
	if err != nil {
		t.Fatalf("ToMarkdown: %v", err)
	}
	for i, want := range []string{"One", "Two", "Three"} {
		if !strings.Contains(markdowns[i], want) {
			t.Errorf("markdowns[%d] = %q, want it to contain %q", i, markdowns[i], want)
		}
	}
}

func TestToBodyBlankMarkdownYieldsEmptyDocument(t *testing.T) {
	requireNode(t)

	bodies, err := (&Converter{}).ToBody(context.Background(), []string{""})
	if err != nil {
		t.Fatalf("ToBody: %v", err)
	}

	// The editor cannot open a body with no nodes, so blank markdown must land
	// on one empty paragraph rather than an empty array.
	var nodes []json.RawMessage
	if err := json.Unmarshal([]byte(bodies[0]), &nodes); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, bodies[0])
	}
	if len(nodes) != 1 {
		t.Fatalf("blank markdown produced %d nodes, want 1: %s", len(nodes), bodies[0])
	}
}

func TestToBodyEmptySliceSkipsSubprocess(t *testing.T) {
	// No node needed: an empty batch must never spawn a process, which the
	// bogus NodePath proves.
	converter := &Converter{NodePath: filepath.Join(t.TempDir(), "no-such-node")}

	bodies, err := converter.ToBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("ToBody: %v", err)
	}
	if len(bodies) != 0 {
		t.Errorf("got %d bodies, want 0", len(bodies))
	}
}

func TestToMarkdownSkipsBlankBodies(t *testing.T) {
	requireNode(t)

	body, err := (&Converter{}).ToBody(context.Background(), []string{"# Title"})
	if err != nil {
		t.Fatalf("ToBody: %v", err)
	}

	markdowns, err := (&Converter{}).ToMarkdown(context.Background(), []string{"", body[0], "   "})
	if err != nil {
		t.Fatalf("ToMarkdown: %v", err)
	}
	if len(markdowns) != 3 {
		t.Fatalf("got %d markdowns, want 3", len(markdowns))
	}
	// Blank bodies are dropped from the batch, so the converted result has to be
	// written back to its original index, not the batch's.
	if markdowns[0] != "" || markdowns[2] != "" {
		t.Errorf("blank bodies converted to %q and %q, want empty strings", markdowns[0], markdowns[2])
	}
	if !strings.Contains(markdowns[1], "# Title") {
		t.Errorf("markdowns[1] = %q, want it to contain %q", markdowns[1], "# Title")
	}
}

func TestToMarkdownAllBlankSkipsSubprocess(t *testing.T) {
	converter := &Converter{NodePath: filepath.Join(t.TempDir(), "no-such-node")}

	markdowns, err := converter.ToMarkdown(context.Background(), []string{"", "  "})
	if err != nil {
		t.Fatalf("ToMarkdown: %v", err)
	}
	if len(markdowns) != 2 || markdowns[0] != "" || markdowns[1] != "" {
		t.Errorf("got %#v, want two empty strings", markdowns)
	}
}

// A conversion failure must reach the caller. Returning the input unchanged
// would write the raw Plate JSON into a task body as if it were markdown.
func TestToMarkdownMalformedBodyErrors(t *testing.T) {
	requireNode(t)

	markdowns, err := (&Converter{}).ToMarkdown(context.Background(), []string{"not json at all"})
	if err == nil {
		t.Fatalf("expected an error, got %#v", markdowns)
	}
	if markdowns != nil {
		t.Errorf("got %#v alongside the error, want nil", markdowns)
	}
	if !strings.Contains(err.Error(), "toMarkdown") {
		t.Errorf("error %q does not name the failing direction", err)
	}
	// The script reports which item of the batch failed; losing that detail
	// would make a bad body in a 25-result search impossible to find.
	if !strings.Contains(err.Error(), "item 0") {
		t.Errorf("error %q does not identify the failing item", err)
	}
}

func TestMissingNodeErrors(t *testing.T) {
	converter := &Converter{NodePath: filepath.Join(t.TempDir(), "no-such-node")}

	_, err := converter.ToBody(context.Background(), []string{"# Title"})
	if err == nil {
		t.Fatal("expected an error when the node binary does not exist")
	}
	if !strings.Contains(err.Error(), "no-such-node") {
		t.Errorf("error %q does not name the missing binary", err)
	}
}

func TestTimeoutErrors(t *testing.T) {
	requireNode(t)

	converter := &Converter{Timeout: time.Nanosecond}

	_, err := converter.ToBody(context.Background(), []string{sampleMarkdown})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error %q is not reported as a timeout", err)
	}
}

func TestCallerContextCancellationErrors(t *testing.T) {
	requireNode(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (&Converter{}).ToBody(ctx, []string{sampleMarkdown}); err == nil {
		t.Fatal("expected an error when the caller's context is already cancelled")
	}
}
