package db

import "testing"

// TestIsWritableSessionKind pins the writable set. "schedule" is in it on
// purpose: a schedule session is the agent's single long-lived cron thread, and
// the user must be able to keep talking in it after a scheduled turn lands.
// The orchestrator-owned run logs — a finished
// record with no turn to attach to — stay closed to new user turns.
func TestIsWritableSessionKind(t *testing.T) {
	for _, kind := range []string{"", "chat", "spawned", "schedule", "automation-run", "schedule-run"} {
		if !IsWritableSessionKind(kind) {
			t.Errorf("kind %q must be writable", kind)
		}
	}
	for _, kind := range []string{"task", "flow", "automation", "worker", SessionKindInsight} {
		if IsWritableSessionKind(kind) {
			t.Errorf("kind %q must not be writable", kind)
		}
	}
}

// TestScheduleKindIsWritableButNotImmutable: making the cron thread writable must
// not have dragged it into the immutable set (or out of it) by accident.
func TestScheduleKindIsWritableButNotImmutable(t *testing.T) {
	if IsImmutableSessionKind("schedule") {
		t.Fatal("a schedule transcript is not immutable — the scheduler appends to it")
	}
}
