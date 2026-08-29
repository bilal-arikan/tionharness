package agent

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func multiLensRequest(transcript string) insight.AnalysisRequest {
	lenses := []insight.Lens{
		{ID: "tool-errors", Channel: insight.ChannelAppFix, Prompt: "Find tool errors."},
		{ID: "context-hygiene", Channel: insight.ChannelWorkspaceOpt, Prompt: "Find context bloat."},
	}
	return insight.AnalysisRequest{Lens: lenses[0], Lenses: lenses, SessionID: "SES1", Transcript: transcript}
}

// TestAnalysisUserPromptMultiLens: one grouped call must carry every lens's
// instruction and demand a lensId, otherwise the scanner cannot attribute the
// findings it gets back.
func TestAnalysisUserPromptMultiLens(t *testing.T) {
	got := analysisUserPrompt(multiLensRequest("some evidence"), "", nil)
	for _, want := range []string{
		"Find tool errors.", "Find context bloat.",
		"LENS tool-errors", "LENS context-hygiene",
		"\"lensId\"", "lensId must be exactly one of: tool-errors, context-hygiene.",
		"some evidence",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("grouped prompt missing %q:\n%s", want, got)
		}
	}

	// A single-lens call stays clean: no lensId ceremony where nothing is ambiguous.
	solo := analysisUserPrompt(insight.AnalysisRequest{
		Lens: insight.Lens{ID: "tool-errors", Prompt: "Find tool errors."}, Transcript: "e",
	}, "", nil)
	if strings.Contains(solo, "lensId") {
		t.Fatalf("single-lens prompt should not ask for a lensId:\n%s", solo)
	}
}

// TestAnalysisUserPromptEvidenceIsLast pins the prompt-cache contract: the
// per-session evidence must sit AFTER the run-stable lens instructions, so
// consecutive calls of one run share a cacheable prefix. Evidence-first would
// give them no common prefix at all.
func TestAnalysisUserPromptEvidenceIsLast(t *testing.T) {
	got := analysisUserPrompt(multiLensRequest("EVIDENCE-BODY"), "Turkish (Türkçe)", []string{"sig-1"})
	evidence := strings.Index(got, "--- SESSION EVIDENCE ---")
	if evidence < 0 {
		t.Fatalf("no evidence section:\n%s", got)
	}
	for _, before := range []string{"LENS tool-errors", "Return ONLY JSON", "Write the title", "--- SIGNATURES ALREADY ON RECORD ---"} {
		if idx := strings.Index(got, before); idx < 0 || idx > evidence {
			t.Fatalf("%q must come before the evidence (idx=%d, evidence=%d):\n%s", before, idx, evidence, got)
		}
	}
	if !strings.HasSuffix(got, "EVIDENCE-BODY") {
		t.Fatalf("the evidence must be the LAST block:\n%s", got)
	}

	// Two sessions of the same run differ only in their tail.
	other := analysisUserPrompt(multiLensRequest("DIFFERENT-BODY"), "Turkish (Türkçe)", []string{"sig-1"})
	if !strings.HasPrefix(other, got[:evidence]) {
		t.Fatal("two calls of one run must share the whole pre-evidence prefix")
	}
}

// TestAnalysisMaxTokensScalesWithGroup: a grouped call answers for several lenses
// in one reply, so its budget must grow — and stay bounded.
func TestAnalysisMaxTokensScalesWithGroup(t *testing.T) {
	if got := analysisMaxTokens(1); got != 1500 {
		t.Fatalf("single lens budget = %d, want 1500", got)
	}
	if got := analysisMaxTokens(3); got <= analysisMaxTokens(1) {
		t.Fatalf("a 3-lens call must get more room than a 1-lens call, got %d", got)
	}
	if got := analysisMaxTokens(8); got != 5000 {
		t.Fatalf("default 8-lens group budget = %d, want 5000", got)
	}
}

func TestInsightAnalyzerParseErrorsAreReturned(t *testing.T) {
	a := &insightAnalyzer{rt: &Runtime{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	lenses := multiLensRequest("evidence").LensList()
	for _, reply := range []string{"no json here", `{"lensResults":[}`} {
		if _, err := a.parse(reply, lenses); err == nil {
			t.Fatalf("parse(%q) returned nil error", reply)
		}
	}
}

func TestInsightAnalyzerGroupedEmptyLensIsExplicitlyAnswered(t *testing.T) {
	a := &insightAnalyzer{rt: &Runtime{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	got, err := a.parse(`{"lensResults":[{"lensId":"tool-errors","findings":[]}]}`, multiLensRequest("evidence").LensList())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].AnswerOnly || got[0].LensID != "tool-errors" {
		t.Fatalf("empty lens result marker = %+v", got)
	}
}

func TestInsightAnalyzerRejectsMaxTokensStop(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	provider.text = `{"lensResults":[{"lensId":"tool-errors","findings":[]}]}`
	provider.stop = providers.StopMaxTok
	a := &insightAnalyzer{
		rt:    rt,
		agent: db.Agent{Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "test-model"},
	}
	_, err := a.Analyze(context.Background(), multiLensRequest("evidence"))
	if err == nil {
		t.Fatal("Analyze accepted max_tokens stop")
	}
	for _, want := range []string{"2 lenses", "maxTokens=2000", "stopReason=max_tokens"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}
