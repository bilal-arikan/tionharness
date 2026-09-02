package agent

import (
	"testing"
	"time"
)

// The breaker opens exactly at the threshold, stays open for the cooldown,
// lets ONE probe through afterwards, and a failed probe re-opens it.
func TestMCPFailBreakerOpensAtThresholdAndProbesAfterCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := mcpFailStreaks{now: func() time.Time { return now }}

	for i := int64(1); i < mcpFailStreakThreshold; i++ {
		s.note("cbm")
		if isOpen, _, _ := s.open("cbm"); isOpen {
			t.Fatalf("breaker open after %d failure(s), want closed below the threshold", i)
		}
	}
	s.note("cbm")
	isOpen, retryIn, streak := s.open("cbm")
	if !isOpen || streak != mcpFailStreakThreshold || retryIn != mcpBreakerCooldown {
		t.Fatalf("at threshold: open=%v retryIn=%s streak=%d", isOpen, retryIn, streak)
	}

	// Mid-cooldown: still open, remaining time shrinks.
	now = now.Add(mcpBreakerCooldown / 2)
	if isOpen, retryIn, _ := s.open("cbm"); !isOpen || retryIn != mcpBreakerCooldown/2 {
		t.Fatalf("mid-cooldown: open=%v retryIn=%s", isOpen, retryIn)
	}

	// Cooldown over: closed for the probe, streak preserved.
	now = now.Add(mcpBreakerCooldown / 2)
	if isOpen, _, streak := s.open("cbm"); isOpen || streak != mcpFailStreakThreshold {
		t.Fatalf("after cooldown: open=%v streak=%d, want closed with the streak kept", isOpen, streak)
	}

	// The probe fails: re-opened for a full cooldown at streak+1.
	s.note("cbm")
	if isOpen, retryIn, streak := s.open("cbm"); !isOpen || retryIn != mcpBreakerCooldown || streak != mcpFailStreakThreshold+1 {
		t.Fatalf("after failed probe: open=%v retryIn=%s streak=%d", isOpen, retryIn, streak)
	}

	// Success closes it and resets the streak.
	s.clear("cbm")
	if isOpen, _, streak := s.open("cbm"); isOpen || streak != 0 {
		t.Fatalf("after clear: open=%v streak=%d", isOpen, streak)
	}
}

func TestMCPFailBreakerUnknownServerIsClosed(t *testing.T) {
	var s mcpFailStreaks
	if isOpen, retryIn, streak := s.open("never"); isOpen || retryIn != 0 || streak != 0 {
		t.Fatalf("unknown server: open=%v retryIn=%s streak=%d", isOpen, retryIn, streak)
	}
}
