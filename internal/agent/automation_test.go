package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func TestRenderAutomationPrompt(t *testing.T) {
	sess := db.Session{ID: "SES3", Title: "My loop"}

	// All placeholders substituted.
	got := renderAutomationPrompt("[{{tag}}] {{title}} ({{sessionId}}): {{result}}", "DONE", sess, "loop")
	want := "[loop] My loop (SES3): DONE"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// A template without {{result}} still carries the result forward (appended).
	got = renderAutomationPrompt("Keep going.", "the result", sess, "loop")
	if !strings.Contains(got, "the result") || !strings.HasPrefix(got, "Keep going.") {
		t.Fatalf("result not appended: %q", got)
	}

	// Empty result + no placeholder → template unchanged (nothing appended).
	got = renderAutomationPrompt("Just do X.", "", sess, "loop")
	if got != "Just do X." {
		t.Fatalf("unexpected mutation: %q", got)
	}
}

func TestContainsTag(t *testing.T) {
	if !containsTag([]string{"a", "loop", "b"}, "loop") {
		t.Fatal("should find loop")
	}
	if containsTag([]string{"a", "b"}, "loop") {
		t.Fatal("should not find loop")
	}
	if containsTag(nil, "loop") {
		t.Fatal("nil should not match")
	}
}
