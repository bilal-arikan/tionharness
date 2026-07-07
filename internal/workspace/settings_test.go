package workspace

import "testing"

// TestDefaultWSSettings pins the cross-session awareness defaults: on, first-turn
// only, 5 recent sessions.
func TestDefaultWSSettings(t *testing.T) {
	d := defaultWSSettings()
	if !d.SessionContextEnabled {
		t.Error("session context should default on")
	}
	if d.SessionContextEveryTurn {
		t.Error("every-turn should default off (first turn only)")
	}
	if d.SessionContextRecentCount != 5 {
		t.Errorf("recent count default = %d, want 5", d.SessionContextRecentCount)
	}
	if !d.CodebaseMemoryEnabled {
		t.Error("codebase-memory capability should default on")
	}
}

// TestClampRecent bounds the recent count to [1,20].
func TestClampRecent(t *testing.T) {
	cases := map[int]int{0: 1, -3: 1, 1: 1, 7: 7, 20: 20, 99: 20}
	for in, want := range cases {
		if got := clampRecent(in); got != want {
			t.Errorf("clampRecent(%d) = %d, want %d", in, got, want)
		}
	}
}
