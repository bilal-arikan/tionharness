package agent

import "testing"

// TestContinuationKind pins the handoff-continuation classification: a chat
// parent (or legacy "") keeps the continuation in the "Sohbet" sidebar filter,
// while every other parent kind falls back to the writable "spawned" kind so a
// non-writable/tree kind never leaks onto the human-continuable continuation.
func TestContinuationKind(t *testing.T) {
	cases := map[string]string{
		"":                 "chat",
		"chat":             "chat",
		" chat ":           "chat",
		"spawned":          "spawned",
		"task":             "spawned",
		"flow":             "spawned",
		"schedule":         "spawned",
		"worker":           "spawned",
		"flow-coordinator": "spawned",
	}
	for parent, want := range cases {
		if got := continuationKind(parent); got != want {
			t.Errorf("continuationKind(%q) = %q, want %q", parent, got, want)
		}
	}
}
