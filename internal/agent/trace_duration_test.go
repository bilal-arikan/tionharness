package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestTraceStepKeepsClaudeToolLatency: the claude-cli parser measures tool
// latency into DurMs only; the persisted step must still carry it.
func TestTraceStepKeepsClaudeToolLatency(t *testing.T) {
	st := traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "Bash", DurMs: 1234})
	if st.DurationMs != 1234 {
		t.Fatalf("DurationMs = %d, want 1234 from DurMs", st.DurationMs)
	}
	st = traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "Bash", DurMs: 5, DurationMs: 99})
	if st.DurationMs != 99 {
		t.Fatalf("an explicit DurationMs must win, got %d", st.DurationMs)
	}
}
