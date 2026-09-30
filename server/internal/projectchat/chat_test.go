package projectchat

import (
	"strings"
	"testing"
)

// excerpt bounds each referenced task's body in the turn payload so a
// handful of #mentions can't balloon the request; it should still read like
// a preview, cutting at a word instead of mid-word.
func TestExcerptBoundsAtAWord(t *testing.T) {
	short := "A short task body."
	if got := excerpt(short, referencedTaskExcerptLength); got != short {
		t.Fatalf("short body was changed: %q", got)
	}
	long := strings.Repeat("word ", 200)
	got := excerpt(long, referencedTaskExcerptLength)
	if len([]rune(got)) > referencedTaskExcerptLength+1 { // +1 for the ellipsis rune
		t.Fatalf("excerpt exceeded its bound: %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated excerpt should end with an ellipsis: %q", got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Fatalf("excerpt should cut at a word, not trail whitespace: %q", got)
	}
	// Collapses newlines and repeated whitespace, like a preview should.
	if got := excerpt("Line one\n\nLine   two", referencedTaskExcerptLength); got != "Line one Line two" {
		t.Fatalf("whitespace wasn't collapsed: %q", got)
	}
}
