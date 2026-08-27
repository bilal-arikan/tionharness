package agent

import "testing"

// TestMCPFailStreakEscalatesOnceThenResets locks the two properties that make the
// escalation useful rather than noisy: the threshold is crossed exactly once per
// outage, and a server that recovers starts over.
func TestMCPFailStreakEscalatesOnceThenResets(t *testing.T) {
	var s mcpFailStreaks

	crossings := 0
	for i := 0; i < mcpFailStreakThreshold*3; i++ {
		if s.note("cbm") == mcpFailStreakThreshold {
			crossings++
		}
	}
	if crossings != 1 {
		t.Fatalf("threshold crossed %d times during one outage, want exactly 1", crossings)
	}

	// Recovery, then a second outage: the threshold must fire again, or a server
	// that flaps all day would escalate only once in the life of the process.
	s.clear("cbm")
	crossings = 0
	for i := 0; i < mcpFailStreakThreshold; i++ {
		if s.note("cbm") == mcpFailStreakThreshold {
			crossings++
		}
	}
	if crossings != 1 {
		t.Fatalf("second outage crossed %d times, want 1", crossings)
	}
}

// TestMCPFailStreakIsPerServer confirms one broken server cannot drag another
// across the threshold — the incident had two servers failing for unrelated reasons.
func TestMCPFailStreakIsPerServer(t *testing.T) {
	var s mcpFailStreaks
	for i := 0; i < mcpFailStreakThreshold; i++ {
		s.note("cbm")
	}
	if got := s.note("playwright"); got != 1 {
		t.Fatalf("playwright streak = %d, want 1 (streaks must not be shared)", got)
	}
	// Clearing one must not clear the other.
	s.clear("playwright")
	if got := s.note("cbm"); got != mcpFailStreakThreshold+1 {
		t.Fatalf("cbm streak = %d, want %d", got, mcpFailStreakThreshold+1)
	}
}

// TestMCPFailStreakClearOnUnknownServerIsSafe: the reset loop runs over every
// configured server, including ones that have never failed.
func TestMCPFailStreakClearOnUnknownServerIsSafe(t *testing.T) {
	var s mcpFailStreaks
	s.clear("never-seen")
	if got := s.note("never-seen"); got != 1 {
		t.Fatalf("streak = %d, want 1", got)
	}
}
