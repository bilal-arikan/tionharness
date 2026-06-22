package conversation

import (
	"context"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

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
