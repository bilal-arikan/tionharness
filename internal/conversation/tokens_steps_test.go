package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestStepsAreNotSentAndNotEstimated pins the reason EstimateTokens counts a
// message's Text only. A turn's Steps trace (tool calls + results) can dwarf its
// text — a real session measured 4.7K chars of text against 236K chars of Steps —
// but toProviderMessages ships Text alone, so the trace is NOT in the model's
// context and must NOT inflate the estimate that gates compaction. Counting it
// would fold history early for tokens that were never spent.
//
// What IS spent is the bounded recent-tool-activity recap the API layer renders
// from these traces; that is counted as its own context bucket
// (api.systemFillers → "Araç çağrıları ve sonuçları"), not here.
func TestStepsAreNotSentAndNotEstimated(t *testing.T) {
	steps := `[{"kind":"tool","tool":"Bash","output":"` + strings.Repeat("x", 60000) + `"}]`
	msgs := []db.Message{
		{Role: "assistant", Text: "done", Steps: steps},
	}

	// The provider messages carry the text only — the trace never leaves the store.
	out := toProviderMessages(context.Background(), msgs)
	if len(out) != 1 {
		t.Fatalf("toProviderMessages returned %d messages, want 1", len(out))
	}
	if strings.Contains(out[0].Text, "kind\":\"tool") || strings.Contains(out[0].Text, strings.Repeat("x", 100)) {
		t.Fatalf("Steps trace leaked into the provider message: %.200q", out[0].Text)
	}

	// Consequently the estimate must stay at text scale, not trace scale.
	withSteps := EstimateTokens("", msgs)
	withoutSteps := EstimateTokens("", []db.Message{{Role: "assistant", Text: "done"}})
	if withSteps != withoutSteps {
		t.Fatalf("EstimateTokens counted the Steps trace: %d with, %d without", withSteps, withoutSteps)
	}
}
