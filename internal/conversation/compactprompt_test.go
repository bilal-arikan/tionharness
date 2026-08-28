package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/prompts"
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

// TestCompactDefaultSections locks the numbered sections of the shipped
// compaction template. Section 9 in particular carries the rules the user set
// mid-conversation; without its own section those constraints get folded into
// "5. Decisions and User Feedback" and paraphrased away over repeated compactions.
func TestCompactDefaultSections(t *testing.T) {
	def := prompts.Default("compact")
	sections := []string{
		"1. Primary Request and Intent:",
		"2. Key Technical Concepts:",
		"3. Files and Code:",
		"4. Errors and Fixes:",
		"5. Decisions and User Feedback:",
		"6. Pending Tasks:",
		"7. Current Work:",
		"8. Next Step:",
		"9. Standing Constraints:",
	}
	for _, s := range sections {
		if !strings.Contains(def, s) {
			t.Errorf("compact default is missing section %q", s)
		}
	}
	// The verbatim rule is the whole point of section 9 — a paraphrasing summary
	// of a prohibition is not the prohibition.
	if !strings.Contains(def, "Reproduce each one verbatim") {
		t.Error("section 9 lost its verbatim-reproduction instruction")
	}
	if err := prompts.Validate("compact", def); err != nil {
		t.Fatalf("compact default invalid after section 9: %v", err)
	}
}
