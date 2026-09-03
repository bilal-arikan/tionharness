package repair

import "testing"

// TestMCPFailStreakEscalatesOnceThenResets locks the two properties that make the
// escalation useful rather than noisy: the threshold is crossed exactly once per
// outage, and a server that recovers starts over.
func TestMCPFailStreakEscalatesOnceThenResets(t *testing.T) {
	var s FailStreaks

	crossings := 0
	for i := 0; i < FailStreakThreshold*3; i++ {
		if s.Note("cbm") == FailStreakThreshold {
			crossings++
		}
	}
	if crossings != 1 {
		t.Fatalf("threshold crossed %d times during one outage, want exactly 1", crossings)
	}

	// Recovery, then a second outage: the threshold must fire again, or a server
	// that flaps all day would escalate only once in the life of the process.
	s.Clear("cbm")
	crossings = 0
	for i := 0; i < FailStreakThreshold; i++ {
		if s.Note("cbm") == FailStreakThreshold {
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
	var s FailStreaks
	for i := 0; i < FailStreakThreshold; i++ {
		s.Note("cbm")
	}
	if got := s.Note("playwright"); got != 1 {
		t.Fatalf("playwright streak = %d, want 1 (streaks must not be shared)", got)
	}
	// Clearing one must not clear the other.
	s.Clear("playwright")
	if got := s.Note("cbm"); got != FailStreakThreshold+1 {
		t.Fatalf("cbm streak = %d, want %d", got, FailStreakThreshold+1)
	}
}

// TestMCPFailStreakClearOnUnknownServerIsSafe: the reset loop runs over every
// configured server, including ones that have never failed.
func TestMCPFailStreakClearOnUnknownServerIsSafe(t *testing.T) {
	var s FailStreaks
	s.Clear("never-seen")
	if got := s.Note("never-seen"); got != 1 {
		t.Fatalf("streak = %d, want 1", got)
	}
}
