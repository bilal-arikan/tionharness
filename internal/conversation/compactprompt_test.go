package conversation

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/prompts"
)

// TestCompactPromptFromCtx covers the safety net around the editable compaction
// prompt: a template carrying both {{summary}} and {{messages}} rides on the
// context, but any malformed edit (a dropped slot, or empty) silently falls
// back to the registry default so the rendered prompt can never lose its data
// slots.
func TestCompactPromptFromCtx(t *testing.T) {
	valid := "Summarize. EXISTING:\n{{summary}}\n\nNEW:\n{{messages}}\nEnd."
	cases := []struct {
		name    string
		tmpl    string
		wantDef bool // expect the registry default (not tmpl)
	}{
		{"valid both slots", valid, false},
		{"summary only", "only {{summary}} here", true},
		{"messages only", "only {{messages}} here", true},
		{"no slots", "no placeholders", true},
		{"empty (no-op)", "", true},
	}
	def := prompts.Default("compact")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := WithCompactPrompt(context.Background(), c.tmpl)
			got := compactPromptFromCtx(ctx)
			isDef := got == def
			if isDef != c.wantDef {
				t.Fatalf("tmpl=%q → default=%v, want default=%v (got %q)", c.tmpl, isDef, c.wantDef, got)
			}
			if !c.wantDef && got != c.tmpl {
				t.Fatalf("valid template not returned verbatim: %q", got)
			}
		})
	}

	// Bare context (no value set) also yields the default.
	if got := compactPromptFromCtx(context.Background()); got != def {
		t.Fatal("bare context should yield the registry default")
	}

	// The registry default itself must validate — otherwise the whole editable
	// mechanism ships broken. (prompts_test.go also locks this for every key.)
	if err := prompts.Validate("compact", def); err != nil {
		t.Fatalf("registry compact default invalid: %v", err)
	}
}
