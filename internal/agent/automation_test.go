package agent

import (
	"strings"
	"testing"
)

func TestRenderAutomationPrompt(t *testing.T) {
	vars := map[string]string{
		"result": "DONE", "title": "My loop", "sessionId": "SES3", "tag": "loop",
		"iteration": "2", "agent": "Bob",
	}

	// All placeholders substituted (including the extended set).
	got := renderAutomationPrompt("[{{tag}}] {{title}} ({{sessionId}}) #{{iteration}} by {{agent}}: {{result}}", vars)
	want := "[loop] My loop (SES3) #2 by Bob: DONE"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// A template without {{result}} still carries the result forward (appended).
	got = renderAutomationPrompt("Keep going.", map[string]string{"result": "the result"})
	if !strings.Contains(got, "the result") || !strings.HasPrefix(got, "Keep going.") {
		t.Fatalf("result not appended: %q", got)
	}

	// Empty result + no placeholder → template unchanged (nothing appended).
	got = renderAutomationPrompt("Just do X.", map[string]string{"result": ""})
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
