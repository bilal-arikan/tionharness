package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func stuckSession(t *testing.T, rt *Runtime) db.Session {
	t.Helper()
	sess, err := rt.db.CreateSession(context.Background(), db.Session{Kind: "chat", AgentID: "AGT1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess
}

func TestStuckCounter_IncrementTagAndReset(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetStuckTurnThreshold(3)
	ctx := context.Background()
	sess := stuckSession(t, rt)

	// Two bad turns: counter grows, no tag yet.
	rt.AutoTagTurn(ctx, sess.ID, nil, "provider_error")
	rt.AutoTagTurn(ctx, sess.ID, nil, "provider_error")
	got, _ := rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 2 {
		t.Fatalf("StuckTurns = %d, want 2", got.StuckTurns)
	}
	if containsTag(got.Tags, TagStuck) {
		t.Fatalf("stuck tag applied below threshold: %v", got.Tags)
	}

	// Third bad turn crosses the threshold → tag.
	rt.AutoTagTurn(ctx, sess.ID, nil, "provider_error")
	got, _ = rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 3 || !containsTag(got.Tags, TagStuck) {
		t.Fatalf("StuckTurns = %d tags = %v, want 3 + stuck tag", got.StuckTurns, got.Tags)
	}

	// A clean turn resets the counter (the tag stays add-only until a fixer).
	rt.AutoTagTurn(ctx, sess.ID, nil, "")
	got, _ = rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 0 {
		t.Fatalf("StuckTurns = %d after clean turn, want 0", got.StuckTurns)
	}
}

func TestStuckCounter_GuardrailHaltCounts(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetStuckTurnThreshold(3)
	ctx := context.Background()
	sess := stuckSession(t, rt)

	// A turn that ended via guardrail halt (no turn error) still counts as bad.
	steps := []TurnStep{{Kind: StepRecovery, Reason: string(termGuardrailHalt), Text: "halted"}}
	rt.AutoTagTurn(ctx, sess.ID, steps, "")
	got, _ := rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 1 {
		t.Fatalf("StuckTurns = %d after guardrail halt, want 1", got.StuckTurns)
	}
}

func TestStuckCounter_UserStopAndGateErrorDoNotCount(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetStuckTurnThreshold(3)
	ctx := context.Background()
	sess := stuckSession(t, rt)

	rt.AutoTagTurn(ctx, sess.ID, nil, "stopped")
	// The gate's own refusal must not deepen the hole.
	rt.AutoTagTurn(ctx, sess.ID, nil, "session SES1 "+stuckGuardMarker+": 3 consecutive failed turns")
	got, _ := rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 0 {
		t.Fatalf("StuckTurns = %d, want 0 (stopped + gate refusal excluded)", got.StuckTurns)
	}
}

func TestStuckGate_RefusesAutonomousTurn(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetStuckTurnThreshold(3)
	ctx := context.Background()
	sess := stuckSession(t, rt)

	if err := rt.db.SetSessionStuckTurns(ctx, sess.ID, 3); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	sctx := WithSessionID(ctx, sess.ID)
	if err := rt.stuckGate(sctx); err == nil || !strings.Contains(err.Error(), stuckGuardMarker) {
		t.Fatalf("stuckGate = %v, want stuck-guard refusal", err)
	}
	// Below threshold: passes.
	_ = rt.db.SetSessionStuckTurns(ctx, sess.ID, 2)
	if err := rt.stuckGate(sctx); err != nil {
		t.Fatalf("stuckGate below threshold = %v, want nil", err)
	}
	// Threshold 0 disables the gate entirely.
	tun.SetStuckTurnThreshold(0)
	_ = rt.db.SetSessionStuckTurns(ctx, sess.ID, 99)
	if err := rt.stuckGate(sctx); err != nil {
		t.Fatalf("stuckGate disabled = %v, want nil", err)
	}
	// No session id on ctx: passes.
	tun.SetStuckTurnThreshold(3)
	if err := rt.stuckGate(ctx); err != nil {
		t.Fatalf("stuckGate without session = %v, want nil", err)
	}
}

func TestRemoveStuckTagResetsCounter(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetStuckTurnThreshold(3)
	ctx := context.Background()
	sess := stuckSession(t, rt)

	_ = rt.db.SetSessionStuckTurns(ctx, sess.ID, 5)
	_ = rt.db.SetSessionTags(ctx, sess.ID, []string{TagStuck, "other"})
	rt.RemoveSessionTags(ctx, sess.ID, []string{TagStuck})
	got, _ := rt.db.GetSession(ctx, sess.ID)
	if got.StuckTurns != 0 {
		t.Fatalf("StuckTurns = %d after stuck untag, want 0", got.StuckTurns)
	}
	if containsTag(got.Tags, TagStuck) || !containsTag(got.Tags, "other") {
		t.Fatalf("tags = %v, want stuck removed and other kept", got.Tags)
	}
}
