package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// stepsFixture builds a Steps trace with n tool steps, each carrying a long
// output — the shape of a tool-heavy assistant turn.
func stepsFixture(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"kind":"tool","tool":"Bash","input":{"command":"go test ./..."},"output":"` +
			strings.Repeat("ok  package/one\\n", 40) + `"}`)
	}
	b.WriteString("]")
	return b.String()
}

// TestToolActivityRecapIsCounted pins the "Araç çağrıları ve sonuçları" bucket:
// the recent-tool-activity recap IS shipped to the provider (composeTurnRequest
// folds it into the dynamic suffix), so its tokens must be counted. Before this
// bucket existed the context meter reported a tool-heavy session as if only the
// message texts were in the window.
func TestToolActivityRecapIsCounted(t *testing.T) {
	history := []db.Message{
		{Role: "user", Text: "run the tests"},
		{Role: "assistant", Text: "done", Steps: stepsFixture(5)},
	}
	block := recentToolActivityBlock(history)
	if strings.TrimSpace(block) == "" {
		t.Fatal("expected a non-empty recent_tool_activity block for a tool-bearing turn")
	}
	if got := countToolRecapLines(block); got != 5 {
		t.Fatalf("countToolRecapLines = %d, want 5\nblock:\n%s", got, block)
	}
	// The recap must be a material share of the window, not a rounding error:
	// far larger than the two short message texts it accompanies.
	recapTokens := conversation.EstimateText(block)
	textTokens := conversation.EstimateText(history[0].Text) + conversation.EstimateText(history[1].Text)
	if recapTokens <= textTokens {
		t.Fatalf("recap tokens %d should exceed message-text tokens %d", recapTokens, textTokens)
	}
}

// TestCountToolRecapLinesIgnoresNonToolTurns guards the empty case: a session
// whose turns ran no tools must not gain a phantom bucket.
func TestCountToolRecapLinesIgnoresNonToolTurns(t *testing.T) {
	history := []db.Message{
		{Role: "user", Text: "hi"},
		{Role: "assistant", Text: "hello", Steps: `[{"kind":"text","text":"hello"}]`},
	}
	if block := recentToolActivityBlock(history); strings.TrimSpace(block) != "" {
		t.Fatalf("expected no recap block for a tool-free turn, got:\n%s", block)
	}
	if got := countToolRecapLines(""); got != 0 {
		t.Fatalf("countToolRecapLines(\"\") = %d, want 0", got)
	}
}
