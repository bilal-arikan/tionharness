package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/insight"
)

// TestAnalysisUserPromptLanguage: the language directive is appended only when a
// language is configured, and always names the user-facing prose fields.
func TestAnalysisUserPromptLanguage(t *testing.T) {
	req := insight.AnalysisRequest{
		Lens:       insight.Lens{Prompt: "Find tool errors."},
		Transcript: "some evidence",
	}

	// No language → no directive (model default).
	if got := analysisUserPrompt(req, "", nil); strings.Contains(got, "Write the title") {
		t.Fatalf("empty language should append no directive:\n%s", got)
	}

	// Configured language → directive naming the prose fields, keeping code verbatim.
	got := analysisUserPrompt(req, "Turkish (Türkçe)", nil)
	for _, want := range []string{"Turkish (Türkçe)", "rootCause", "proposedFix", "verbatim"} {
		if !strings.Contains(got, want) {
			t.Fatalf("directive missing %q:\n%s", want, got)
		}
	}
	// The lens prompt + evidence must still be present.
	if !strings.Contains(got, "Find tool errors.") || !strings.Contains(got, "some evidence") {
		t.Fatalf("prompt dropped lens/evidence:\n%s", got)
	}
}
