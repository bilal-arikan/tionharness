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
	tun.SetWorkdirGuards(true, false)
	if tun.AutonomousBootSeq() {
		t.Errorf("AutonomousBootSeq should be false after SetWorkdirGuards(_, false)")
	}
	tun.SetWorkdirGuards(true, true)
	if !tun.AutonomousBootSeq() {
		t.Errorf("AutonomousBootSeq should be true after SetWorkdirGuards(_, true)")
	}
}

// TestAutonomousBootReminderContent pins the reminder's load-bearing parts:
// the autonomy framing (act, don't ask — no user is watching), baseline
// verification, grounded progress claims, and the playbook skill pointer.
func TestAutonomousBootReminderContent(t *testing.T) {
	for _, want := range []string{
		"Autonomous operation",
		"do not ask permission",
		"verify the baseline",
		"tool result from this session",
		`use_skill "tionswarm-autonomous-ops"`,
	} {
		if !strings.Contains(autonomousBootReminder, want) {
			t.Errorf("autonomousBootReminder missing %q:\n%s", want, autonomousBootReminder)
		}
	}
}
