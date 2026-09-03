package agent

import "testing"

// TestIsAuxiliaryKind locks which call origins count as tool-less side jobs —
// the set that skips the bridge, the built-in menu and the persistent process.
func TestIsAuxiliaryKind(t *testing.T) {
	for _, k := range []CallKind{KindTitle, KindSummary, KindReflect, KindCompact, KindBtw} {
		if !isAuxiliaryKind(k) {
			t.Errorf("%s must be auxiliary", k)
		}
	}
	for _, k := range []CallKind{KindChat, KindTask, KindSchedule, KindFlow, KindSpawn, KindSubagent, KindDelegate} {
		if isAuxiliaryKind(k) {
			t.Errorf("%s must NOT be auxiliary", k)
		}
	}
}
