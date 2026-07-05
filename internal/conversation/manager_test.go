package conversation

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func msgsN(n int) []db.Message {
	out := make([]db.Message, n)
	for i := range out {
		out[i] = db.Message{Role: providers.RoleUser, Text: "m"}
	}
	return out
}

// TestClampStart guards the defensive cap against a history shorter than the
// recorded SummaryMsgCount (e.g. after message edits).
func TestClampStart(t *testing.T) {
	if got := clampStart(5, 10); got != 5 {
		t.Errorf("clampStart(5,10) = %d, want 5", got)
	}
	if got := clampStart(20, 10); got != 10 {
		t.Errorf("clampStart(20,10) = %d, want 10 (clamped)", got)
	}
}

// TestFoldBoundary verifies the shared compaction split used by both Prepare and
// ForceCompact: fold = all but the newest keepRecent, keepTail = the rest, and ok
// is false when there is not enough pending history to fold.
func TestFoldBoundary(t *testing.T) {
	keepRecent := 3

	// 10 messages, none summarized → fold 7, keep 3, newCount 7.
	fold, keep, newCount, ok := foldBoundary(msgsN(10), 0, keepRecent)
	if !ok || len(fold) != 7 || len(keep) != 3 || newCount != 7 {
		t.Errorf("fresh: fold=%d keep=%d newCount=%d ok=%v, want 7/3/7/true", len(fold), len(keep), newCount, ok)
	}

	// 3 already summarized of 10 → pending 7, fold 4, keep 3, newCount 7.
	fold, keep, newCount, ok = foldBoundary(msgsN(10), 3, keepRecent)
	if !ok || len(fold) != 4 || len(keep) != 3 || newCount != 7 {
		t.Errorf("offset: fold=%d keep=%d newCount=%d ok=%v, want 4/3/7/true", len(fold), len(keep), newCount, ok)
	}

	// Exactly / fewer than keepRecent pending → nothing to fold.
	if _, _, _, ok := foldBoundary(msgsN(3), 0, keepRecent); ok {
		t.Error("pending == keepRecent should not fold")
	}
	if _, _, _, ok := foldBoundary(msgsN(2), 0, keepRecent); ok {
		t.Error("pending < keepRecent should not fold")
	}
	// Start beyond history is clamped → no fold, no panic.
	if _, _, _, ok := foldBoundary(msgsN(5), 99, keepRecent); ok {
		t.Error("start beyond history should not fold")
	}
}

// TestPrepareComputesPressure checks the memory-pressure ratio: ContextTokens /
// maxTokens, with a small budget so a couple of messages push it high. Few
// enough messages (<= keepRecent) that no compaction fires, so the stub DB/
// provider are never touched.
func TestPrepareComputesPressure(t *testing.T) {
	m := NewManager()
	m.SetLimits(50, 8) // tiny budget, keepRecent high enough to skip compaction

	history := []db.Message{
		{Role: providers.RoleUser, Text: "hello there, this is a fairly long message"},
		{Role: providers.RoleAssistant, Text: "and here is a long reply to consume tokens"},
	}
	prep, err := m.Prepare(context.Background(), nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prep.Compacted {
		t.Fatalf("did not expect compaction with %d messages under keepRecent", len(history))
	}
	if prep.Pressure <= 0 {
		t.Fatalf("pressure = %f, want > 0", prep.Pressure)
	}
	// Sanity: pressure must equal ContextTokens/maxTokens.
	want := float64(prep.ContextTokens) / 50.0
	if prep.Pressure != want {
		t.Fatalf("pressure = %f, want %f", prep.Pressure, want)
	}
}

// TestPrepareZeroPressureWhenBudgetDisabled ensures a non-positive budget yields
// Pressure 0 (the feature is off) rather than a divide-by-zero.
func TestPrepareZeroPressureWhenBudgetDisabled(t *testing.T) {
	m := &Manager{maxTokens: 0, keepRecent: 8} // construct directly to bypass SetLimits' guard
	history := []db.Message{{Role: providers.RoleUser, Text: "hi"}}
	prep, err := m.Prepare(context.Background(), nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prep.Pressure != 0 {
		t.Fatalf("pressure = %f, want 0 when maxTokens <= 0", prep.Pressure)
	}
}
