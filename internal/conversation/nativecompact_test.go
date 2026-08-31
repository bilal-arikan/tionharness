package conversation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// nativeCompactFixture builds a store, an agent and a session plus a Manager with
// a 1-token budget, so any Prepare over the returned history is over budget and
// the compaction gate always fires.
func nativeCompactFixture(t *testing.T) (*db.DB, *Manager, db.Agent, db.Session, []db.Message) {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	m := NewManager()
	m.SetLimits(1, 2) // budget 1 token forces the gate; keepRecent 2 leaves a fold
	history := []db.Message{
		{Role: providers.RoleUser, Text: "u1"}, {Role: providers.RoleAssistant, Text: "a1"},
		{Role: providers.RoleUser, Text: "u2"}, {Role: providers.RoleAssistant, Text: "a2"},
		{Role: providers.RoleUser, Text: "u3"}, {Role: providers.RoleAssistant, Text: "a3"},
	}
	return d, m, agent, sess, history
}

// TestPrepareRollingModeIgnoresNativeCallback pins the default: in "rolling" mode
// the native seam is never consulted, no matter what is on the context.
func TestPrepareRollingModeIgnoresNativeCallback(t *testing.T) {
	d, m, agent, sess, history := nativeCompactFixture(t)
	calls := 0
	ctx := WithNativeCompact(context.Background(), func(context.Context) error {
		calls++
		return nil
	})
	prep, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if calls != 0 {
		t.Fatalf("native callback called %d times in rolling mode, want 0", calls)
	}
	if !prep.Compacted {
		t.Fatalf("expected the rolling fold to run")
	}
	if prep.NativeCompacted {
		t.Fatalf("NativeCompacted set in rolling mode")
	}
	if prep.Fold.Mode != ModeRolling {
		t.Fatalf("Fold.Mode = %q, want %q", prep.Fold.Mode, ModeRolling)
	}
}

// TestPrepareNativeModeSkipsRollingFold covers the success path: the CLI compacted
// its own window, so TionHarness must NOT also fold history into the summary.
func TestPrepareNativeModeSkipsRollingFold(t *testing.T) {
	d, m, agent, sess, history := nativeCompactFixture(t)
	m.SetAutoCompactMode(AutoCompactNative)
	calls := 0
	ctx := WithNativeCompact(context.Background(), func(context.Context) error {
		calls++
		return nil
	})
	prep, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if calls != 1 {
		t.Fatalf("native callback called %d times, want 1", calls)
	}
	if prep.Compacted {
		t.Fatalf("rolling fold ran after a successful native compaction")
	}
	if !prep.NativeCompacted {
		t.Fatalf("NativeCompacted not reported")
	}
	if prep.Fold.Mode != ModeNative || prep.Fold.Trigger != TriggerAuto {
		t.Fatalf("Fold = %+v, want mode %q trigger %q", prep.Fold, ModeNative, TriggerAuto)
	}
	// The transcript is untouched by native compaction: the summary must be intact.
	after, err := d.GetSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if after.SummaryMsgCount != 0 || after.Summary != "" {
		t.Fatalf("native path moved the rolling boundary: count=%d summary=%q", after.SummaryMsgCount, after.Summary)
	}
	evs, err := d.ReadDebugEvents(context.Background(), sess.ID, db.DebugCompaction, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 1 || evs[0].Name != TriggerAuto+"-"+ModeNative {
		t.Fatalf("debug events = %+v, want one %q event", evs, TriggerAuto+"-"+ModeNative)
	}
}

// TestPrepareNativeFailureFallsBackToRolling covers the fallback: any non-nil
// error from the callback means native compaction did not happen, so the ordinary
// rolling fold must still run on that same turn.
func TestPrepareNativeFailureFallsBackToRolling(t *testing.T) {
	d, m, agent, sess, history := nativeCompactFixture(t)
	m.SetAutoCompactMode(AutoCompactAuto)
	calls := 0
	ctx := WithNativeCompact(context.Background(), func(context.Context) error {
		calls++
		return errors.New("provider does not support native compaction")
	})
	prep, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if calls != 1 {
		t.Fatalf("native callback called %d times, want 1", calls)
	}
	if !prep.Compacted || prep.NativeCompacted {
		t.Fatalf("want rolling fallback, got Compacted=%v NativeCompacted=%v", prep.Compacted, prep.NativeCompacted)
	}
	if prep.Fold.Mode != ModeRolling {
		t.Fatalf("Fold.Mode = %q, want %q", prep.Fold.Mode, ModeRolling)
	}
}

// TestPrepareNativeAntiLoop is the convergence guard. Native compaction shrinks the
// CLI's window but not our transcript, so a second Prepare over the SAME history is
// still over budget. It must not fire a second native compaction — it has to fall
// through to the rolling fold, which actually reduces the text.
func TestPrepareNativeAntiLoop(t *testing.T) {
	d, m, agent, sess, history := nativeCompactFixture(t)
	m.SetAutoCompactMode(AutoCompactNative)
	calls := 0
	ctx := WithNativeCompact(context.Background(), func(context.Context) error {
		calls++
		return nil
	})
	first, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if !first.NativeCompacted || first.Compacted {
		t.Fatalf("first turn: want native only, got native=%v rolling=%v", first.NativeCompacted, first.Compacted)
	}
	second, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if calls != 1 {
		t.Fatalf("native callback called %d times over two identical turns, want 1", calls)
	}
	if second.NativeCompacted {
		t.Fatalf("second turn compacted natively again — the anti-loop guard is not holding")
	}
	if !second.Compacted || second.Fold.Mode != ModeRolling {
		t.Fatalf("second turn: want a rolling fold, got Compacted=%v mode=%q", second.Compacted, second.Fold.Mode)
	}
}
