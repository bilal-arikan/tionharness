package db

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestValidateMaxIterations locks the boundary table for the guard that was
// documented and shipped in the UI on 2026-07-29 but never actually enforced: the
// API accepted 0 without complaint, and four stored automations still carried it
// (three of them enabled and firing). A UI-only rule is not a rule — both the REST
// handler and the agent tool reach this function.
func TestValidateMaxIterations(t *testing.T) {
	rejected := map[int]string{
		0:                        "0 is the old unlimited value — the whole point of the guard",
		-1:                       "negative would also read as unlimited",
		-100:                     "same",
		MaxIterationsHardCap + 1: "one past the ceiling",
		1_000_000:                "the fat-finger case the ceiling exists for",
	}
	for v, why := range rejected {
		err := ValidateMaxIterations(v)
		if err == nil {
			t.Errorf("maxIterations=%d must be rejected (%s)", v, why)
			continue
		}
		// Callers branch on the sentinel; the wrapped text is for the human.
		if !errors.Is(err, ErrMaxIterationsRange) {
			t.Errorf("maxIterations=%d must wrap ErrMaxIterationsRange, got %v", v, err)
		}
	}

	for _, v := range []int{1, 10, 50, MaxIterationsHardCap} {
		if err := ValidateMaxIterations(v); err != nil {
			t.Errorf("maxIterations=%d must be accepted, got %v", v, err)
		}
	}

	// The message has to name the limit — "invalid value" would leave the user
	// guessing at a number only the server knows.
	if err := ValidateMaxIterations(MaxIterationsHardCap + 1); !strings.Contains(err.Error(), strconv.Itoa(MaxIterationsHardCap)) {
		t.Errorf("the ceiling message must state the ceiling, got %q", err)
	}
	if err := ValidateMaxIterations(0); !strings.Contains(err.Error(), "0") {
		t.Errorf("the zero message must explain what is wrong with 0, got %q", err)
	}
}

// TestValidateTokenThreshold locks the floor for a token automation's interval.
// A tiny interval would cross on nearly every call and fire in a tight loop, so
// the same validator guards both the REST handler and the agent tool.
func TestValidateTokenThreshold(t *testing.T) {
	for _, v := range []int{0, 1, MinTokenThreshold - 1, -100} {
		err := ValidateTokenThreshold(v)
		if err == nil {
			t.Errorf("tokenThreshold=%d must be rejected (below floor %d)", v, MinTokenThreshold)
			continue
		}
		if !errors.Is(err, ErrTokenThresholdRange) {
			t.Errorf("tokenThreshold=%d must wrap ErrTokenThresholdRange, got %v", v, err)
		}
	}
	for _, v := range []int{MinTokenThreshold, 100_000, 5_000_000} {
		if err := ValidateTokenThreshold(v); err != nil {
			t.Errorf("tokenThreshold=%d must be accepted, got %v", v, err)
		}
	}
	// The message must name the floor so the user knows the minimum.
	if err := ValidateTokenThreshold(1); !strings.Contains(err.Error(), strconv.Itoa(MinTokenThreshold)) {
		t.Errorf("the floor message must state the floor, got %q", err)
	}
}

// TestValidTokenScope pins the accepted scope set (empty defaults to session).
func TestValidTokenScope(t *testing.T) {
	for _, s := range []string{"", TokenScopeSession, TokenScopeWorkspace} {
		if !ValidTokenScope(s) {
			t.Errorf("scope %q must be valid", s)
		}
	}
	for _, s := range []string{"daily", "agent", "bogus"} {
		if ValidTokenScope(s) {
			t.Errorf("scope %q must be invalid", s)
		}
	}
}

// TestIterationLimitOrdering pins the relationship between the two constants.
// The backstop must sit ABOVE the hard cap: it exists for legacy rows nobody
// bounded, so tripping it earlier than an explicit maximum would punish exactly
// the data that never got a choice.
func TestIterationLimitOrdering(t *testing.T) {
	if AbsoluteIterationBackstop <= MaxIterationsHardCap {
		t.Fatalf("backstop (%d) must exceed the hard cap (%d)", AbsoluteIterationBackstop, MaxIterationsHardCap)
	}
	// And the cap must leave real headroom over the default (50), or the typo guard
	// doubles as a guard against ordinary configuration.
	if MaxIterationsHardCap < 50*5 {
		t.Errorf("hard cap (%d) leaves too little headroom over the default 50", MaxIterationsHardCap)
	}
}
