package api

import "testing"

func TestClaudeResumeDecision(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		cliID       string
		sentCount   int
		rawLen      int
		wantActive  bool
		wantResume  string
		wantDelta   int
		wantSent    int
	}{
		// Gate off → inert (full transcript, no id captured/persisted).
		{"disabled", false, "sess", 2, 5, false, "", 0, 0},
		// Cold start: no prior id → send full transcript, persist count for next turn.
		{"cold first turn", true, "", 0, 1, true, "", 0, 1},
		// Warm resume: prior id + valid boundary + unseen delta.
		{"warm with delta", true, "sess", 3, 5, true, "sess", 3, 5},
		// Boundary equals raw length → nothing new → cold fallback (re-capture id).
		{"no new messages", true, "sess", 5, 5, true, "", 0, 5},
		// Boundary past the end (history shrank after edits) → cold fallback.
		{"boundary past end", true, "sess", 9, 5, true, "", 0, 5},
		// Prior id but zero boundary (shouldn't happen) → cold.
		{"zero boundary", true, "sess", 0, 4, true, "", 0, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, resumeID, delta := claudeResumeDecision(c.enabled, c.cliID, c.sentCount, c.rawLen)
			if plan.active != c.wantActive {
				t.Errorf("active = %v, want %v", plan.active, c.wantActive)
			}
			if resumeID != c.wantResume {
				t.Errorf("resumeID = %q, want %q", resumeID, c.wantResume)
			}
			if delta != c.wantDelta {
				t.Errorf("deltaStart = %d, want %d", delta, c.wantDelta)
			}
			if c.wantActive && plan.sentCount != c.wantSent {
				t.Errorf("sentCount = %d, want %d", plan.sentCount, c.wantSent)
			}
		})
	}
}
