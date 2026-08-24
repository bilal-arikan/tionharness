package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/prompts"
)

func TestCapTextUnderLimitUnchanged(t *testing.T) {
	s := "short result"
	got, truncated := capText(s, coordinatorResultCapChars)
	if truncated {
		t.Fatalf("did not expect truncation for a short string")
	}
	if got != s {
		t.Fatalf("string altered: %q", got)
	}
}

func TestCapTextTruncatesAndAnnotates(t *testing.T) {
	s := strings.Repeat("a", coordinatorResultCapChars+500)
	got, truncated := capText(s, coordinatorResultCapChars)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	if !strings.Contains(got, "karakter kırpıldı") {
		t.Fatalf("missing truncation notice: %q", got[len(got)-60:])
	}
	// The kept prefix must be exactly the cap in RUNES (the notice is extra).
	head := got[:strings.Index(got, "\n\n…")]
	if n := len([]rune(head)); n != coordinatorResultCapChars {
		t.Fatalf("kept %d runes, want %d", n, coordinatorResultCapChars)
	}
}

// A byte-offset cap would split a multi-byte rune and corrupt Turkish text; capText
// must slice on runes. Fill past the cap with 2-byte runes and assert the boundary
// is a clean rune.
func TestCapTextIsRuneSafe(t *testing.T) {
	s := strings.Repeat("ğ", coordinatorResultCapChars+200)
	got, truncated := capText(s, coordinatorResultCapChars)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	head := got[:strings.Index(got, "\n\n…")]
	for _, r := range head {
		if r != 'ğ' {
			t.Fatalf("rune boundary corrupted: found %q", r)
		}
	}
}

func TestValidatorProfileRegistered(t *testing.T) {
	p, ok := defaultSubagentProfiles["validator"]
	if !ok {
		t.Fatalf("validator profile missing")
	}
	// It must be able to run the codebase but not edit it.
	tools := strings.Join(p.AllowedTools, ",")
	if !strings.Contains(tools, "Bash") {
		t.Fatalf("validator needs Bash to run tests; got %v", p.AllowedTools)
	}
	if strings.Contains(tools, "Write") || strings.Contains(tools, "Edit") {
		t.Fatalf("validator must not edit source; got %v", p.AllowedTools)
	}
	if strings.TrimSpace(prompts.Default("subagent-validator")) == "" {
		t.Fatalf("subagent-validator prompt default is empty")
	}
}
