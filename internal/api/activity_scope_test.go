package api

import (
	"sort"
	"testing"
)

// TestActiveSessionIDsWorkspaceScope locks the workspace-isolation fix for the
// server-wide chatRuns registry: activeSessionIDs must only report turns that
// belong to the requested workspace, so switching into an idle workspace does
// not inherit another workspace's "busy" nav indicators. Passing "" stays
// unscoped (legacy/test behaviour).
func TestActiveSessionIDsWorkspaceScope(t *testing.T) {
	runs := newChatRuns()
	runs.register("rA", "sA", "wsA", func() {})
	runs.register("rB", "sB", "wsB", func() {})
	runs.register("rA2", "sA2", "wsA", func() {})

	got := runs.activeSessionIDs("wsA")
	sort.Strings(got)
	if want := []string{"sA", "sA2"}; !equalStrs(got, want) {
		t.Fatalf("wsA scope = %v, want %v", got, want)
	}

	if got := runs.activeSessionIDs("wsB"); !equalStrs(got, []string{"sB"}) {
		t.Fatalf("wsB scope = %v, want [sB]", got)
	}

	// A workspace with no in-flight turn must report nothing (the bug: it used
	// to inherit the other workspaces' running sessions).
	if got := runs.activeSessionIDs("wsC"); len(got) != 0 {
		t.Fatalf("idle wsC scope = %v, want empty", got)
	}

	// Unscoped ("") returns every workspace's sessions.
	all := runs.activeSessionIDs("")
	sort.Strings(all)
	if want := []string{"sA", "sA2", "sB"}; !equalStrs(all, want) {
		t.Fatalf("unscoped = %v, want %v", all, want)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
