package conversation

import (
	"context"
	"strings"
	"testing"
)

// TestCompactPromptFromCtx covers the safety net around the editable compaction
// prompt: a valid two-%s template rides on the context, but any malformed edit
// (wrong slot count, a stray % verb, or empty) silently falls back to the
// compiled-in default so fmt.Sprintf can never emit a "%!"-marked broken prompt.
func TestCompactPromptFromCtx(t *testing.T) {
	valid := "Summarize. EXISTING:\n%s\n\nNEW:\n%s\nEnd."
	cases := []struct {
		name    string
		tmpl    string
		wantDef bool // expect the compiled-in default (not tmpl)
	}{
		{"valid two slots", valid, false},
		{"one slot", "only %s here", true},
		{"three slots", "%s %s %s", true},
		{"stray percent", "100% done %s %s", true}, // 3 '%' total → invalid
		{"no slots", "no placeholders", true},
		{"empty (no-op)", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := WithCompactPrompt(context.Background(), c.tmpl)
			got := compactPromptFromCtx(ctx)
			isDef := got == compactPrompt
			if isDef != c.wantDef {
				t.Fatalf("tmpl=%q → default=%v, want default=%v (got %q)", c.tmpl, isDef, c.wantDef, got)
			}
			if !c.wantDef && got != c.tmpl {
				t.Fatalf("valid template not returned verbatim: %q", got)
			}
		})
	}

	// Bare context (no value set) also yields the default.
	if got := compactPromptFromCtx(context.Background()); got != compactPrompt {
		t.Fatal("bare context should yield the compiled-in default")
	}

	// The compiled-in default itself must be a valid two-slot template — otherwise
	// the whole editable mechanism ships broken.
	if strings.Count(compactPrompt, "%s") != 2 || strings.Count(compactPrompt, "%") != 2 {
		t.Fatalf("compiled-in compactPrompt is not a clean two-%%s template")
	}
}
