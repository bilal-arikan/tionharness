package repair

import (
	"testing"
	"time"
)

// The breaker opens on the FIRST failure, stays open for the cooldown, lets ONE
// probe through afterwards, and a failed probe re-opens it.
//
// Opening immediately is the point: a server hung in initialize costs a full dial
// deadline per build, so waiting for a streak of threshold before skipping burned
// threshold x DefaultDialTimeout of blocked UI at project open.
func TestMCPFailBreakerOpensOnFirstFailureAndProbesAfterCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := FailStreaks{now: func() time.Time { return now }}

	s.Note("cbm")
	isOpen, retryIn, streak := s.Open("cbm")
	if !isOpen || streak != 1 || retryIn != BreakerCooldown {
		t.Fatalf("after first failure: open=%v retryIn=%s streak=%d, want open at streak 1", isOpen, retryIn, streak)
	}

	// Each further failure re-arms a full cooldown and keeps counting toward the
	// ERROR-escalation threshold, which is independent of the breaker window.
	for i := int64(2); i <= FailStreakThreshold; i++ {
		now = now.Add(BreakerCooldown)
		s.Note("cbm")
		if isOpen, retryIn, streak := s.Open("cbm"); !isOpen || retryIn != BreakerCooldown || streak != i {
			t.Fatalf("failure %d: open=%v retryIn=%s streak=%d", i, isOpen, retryIn, streak)
		}
	}

	// Mid-cooldown: still open, remaining time shrinks.
	now = now.Add(BreakerCooldown / 2)
	if isOpen, retryIn, _ := s.Open("cbm"); !isOpen || retryIn != BreakerCooldown/2 {
		t.Fatalf("mid-cooldown: open=%v retryIn=%s", isOpen, retryIn)
	}

	// Cooldown over: closed for the probe, streak preserved.
	now = now.Add(BreakerCooldown / 2)
	if isOpen, _, streak := s.Open("cbm"); isOpen || streak != FailStreakThreshold {
		t.Fatalf("after cooldown: open=%v streak=%d, want closed with the streak kept", isOpen, streak)
	}

	// The probe fails: re-opened for a full cooldown at streak+1.
	s.Note("cbm")
	if isOpen, retryIn, streak := s.Open("cbm"); !isOpen || retryIn != BreakerCooldown || streak != FailStreakThreshold+1 {
		t.Fatalf("after failed probe: open=%v retryIn=%s streak=%d", isOpen, retryIn, streak)
	}

	// Success closes it and resets the streak.
	s.Clear("cbm")
	if isOpen, _, streak := s.Open("cbm"); isOpen || streak != 0 {
		t.Fatalf("after clear: open=%v streak=%d", isOpen, streak)
	}
}

func TestMCPFailBreakerUnknownServerIsClosed(t *testing.T) {
	var s FailStreaks
	if isOpen, retryIn, streak := s.Open("never"); isOpen || retryIn != 0 || streak != 0 {
		t.Fatalf("unknown server: open=%v retryIn=%s streak=%d", isOpen, retryIn, streak)
	}
}
