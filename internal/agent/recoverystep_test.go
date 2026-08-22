package agent

import (
	"strings"
	"testing"
)

// TestMCPRepairStep asserts the inline card the loop emits alongside the debug
// journal for an MCP argument-repair episode: stable machine reason, non-empty
// human text naming the subject, and the tool batch carried through.
func TestMCPRepairStep(t *testing.T) {
	cases := []struct {
		name       string
		reason     string
		detail     string
		batch      int
		wantSubstr string
	}{
		{
			name:       "auto-corrected argument",
			reason:     reasonMCPRepairRetry,
			detail:     searchTool,
			batch:      2,
			wantSubstr: searchTool,
		},
		{
			name:       "background index started",
			reason:     reasonMCPRepairIndex,
			detail:     tionswarmCwd,
			batch:      0,
			wantSubstr: tionswarmCwd,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := mcpRepairStep(tc.reason, tc.detail, tc.batch)
			if st.Kind != StepRecovery {
				t.Errorf("kind = %q, want %q", st.Kind, StepRecovery)
			}
			if st.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", st.Reason, tc.reason)
			}
			if st.Batch != tc.batch {
				t.Errorf("batch = %d, want %d", st.Batch, tc.batch)
			}
			if !strings.Contains(st.Text, tc.wantSubstr) {
				t.Errorf("text %q does not mention %q", st.Text, tc.wantSubstr)
			}
		})
	}
}

// TestRecoveryTextCompaction guards the reason tag and text of the compaction
// card the loop emits on a SUCCESSFUL in-flight compaction (the exhausted case
// has its own tag), so the two stay distinguishable in the trace.
func TestRecoveryTextCompaction(t *testing.T) {
	if string(contCompactRetry) == string(termContextExhausted) {
		t.Fatalf("compaction and exhaustion must not share a reason tag")
	}
	if recoveryText(contCompactRetry) == "" {
		t.Error("compaction recovery card has no human text")
	}
}
