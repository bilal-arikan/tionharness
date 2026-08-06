package api

import "testing"

// TestResumeGateEnabled locks the ClaudeResume ⟂ ClaudePersistentSession contract:
// persistent-session ALWAYS supersedes --resume, and the delta path is claude-cli +
// single-agent only. Regression guard for the "both settings on → neither trims"
// gotcha (see _Docs/17).
func TestResumeGateEnabled(t *testing.T) {
	cases := []struct {
		name       string
		resume     bool
		persistent bool
		agentCount int
		provider   string
		multiPart  bool
		want       bool
	}{
		{"resume only, single claude-cli", true, false, 1, "claude-cli", false, true},
		{"persistent supersedes resume", true, true, 1, "claude-cli", false, false},
		{"persistent only", false, true, 1, "claude-cli", false, false},
		{"resume off", false, false, 1, "claude-cli", false, false},
		{"multi-agent turn blocks resume", true, false, 2, "claude-cli", false, false},
		{"multi-participant session blocks resume", true, false, 1, "claude-cli", true, false},
		{"non-cli provider blocks resume", true, false, 1, "anthropic", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resumeGateEnabled(c.resume, c.persistent, c.agentCount, c.provider, c.multiPart); got != c.want {
				t.Errorf("resumeGateEnabled(%v,%v,%d,%q,%v) = %v, want %v",
					c.resume, c.persistent, c.agentCount, c.provider, c.multiPart, got, c.want)
			}
		})
	}
}

func TestClaudeResumeDecision(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		cliID      string
		sentCount  int
		rawLen     int
		compacted  bool
		wantActive bool
		wantResume string
		wantDelta  int
		wantSent   int
	}{
		// Gate off → inert (full transcript, no id captured/persisted).
		{"disabled", false, "sess", 2, 5, false, false, "", 0, 0},
		// Cold start: no prior id → send full transcript, persist count for next turn.
		{"cold first turn", true, "", 0, 1, false, true, "", 0, 1},
		// Warm resume: prior id + valid boundary + unseen delta.
		{"warm with delta", true, "sess", 3, 5, false, true, "sess", 3, 5},
		// Boundary equals raw length → nothing new → cold fallback (re-capture id).
		{"no new messages", true, "sess", 5, 5, false, true, "", 0, 5},
		// Boundary past the end (history shrank after edits) → cold fallback.
		{"boundary past end", true, "sess", 9, 5, false, true, "", 0, 5},
		// Prior id but zero boundary (shouldn't happen) → cold.
		{"zero boundary", true, "sess", 0, 4, false, true, "", 0, 4},
		// A fold this turn forces a COLD start even with a valid warm boundary, so the
		// compacted tail re-baselines a fresh CLI session; sentCount stays rawLen.
		{"compacted forces cold despite warm boundary", true, "sess", 3, 5, true, true, "", 0, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, resumeID, delta := claudeResumeDecision(c.enabled, c.cliID, c.sentCount, c.rawLen, c.compacted)
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
