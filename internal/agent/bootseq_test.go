package agent

import (
	"strings"
	"testing"
)

// TestAutonomousBootSeqDefaultOn verifies the boot-sequence gate defaults on and
// follows SetWorkdirGuards, so the settings toggle actually reaches the runtime.
func TestAutonomousBootSeqDefaultOn(t *testing.T) {
	tun := NewTunables()
	if !tun.AutonomousBootSeq() {
		t.Fatalf("AutonomousBootSeq should default to true")
	}

	// SetWorkdirGuards is the single settings → tunables bridge for the autonomous
	// guards; flipping the boot-seq flag must take effect.
	tun.SetWorkdirGuards(true, false, false)
	if tun.AutonomousBootSeq() {
		t.Errorf("AutonomousBootSeq should be false after SetWorkdirGuards(_, _, false)")
	}
	tun.SetWorkdirGuards(true, false, true)
	if !tun.AutonomousBootSeq() {
		t.Errorf("AutonomousBootSeq should be true after SetWorkdirGuards(_, _, true)")
	}
}

// TestAutonomousBootReminderContent pins the reminder's load-bearing parts: the
// boot-sequence header and the pointer to the full recipe skill.
func TestAutonomousBootReminderContent(t *testing.T) {
	for _, want := range []string{
		"Autonomous boot sequence",
		"verify the baseline",
		`use_skill "swarmgo-autonomous-ops"`,
	} {
		if !strings.Contains(autonomousBootReminder, want) {
			t.Errorf("autonomousBootReminder missing %q:\n%s", want, autonomousBootReminder)
		}
	}
}
