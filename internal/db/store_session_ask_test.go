package db

import (
	"context"
	"testing"
)

// TestClaimSessionAsk_CASGuardsDoubleAnswer verifies the durable-ask single-winner
// CAS: the first answerer transitions waiting → resolved and stamps the answer; a
// concurrent second answer fails because the ask is no longer waiting.
func TestClaimSessionAsk_CASGuardsDoubleAnswer(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := d.CreateSessionAsk(ctx, SessionAsk{SessionID: "SES1", AgentID: "AGT1", CallID: "call-1", State: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != SessionAskWaiting {
		t.Fatalf("new ask should be waiting, got %q", a.Status)
	}

	won, err := d.ClaimSessionAsk(ctx, a.ID, "yes")
	if err != nil {
		t.Fatalf("first claim should win: %v", err)
	}
	if won.Status != SessionAskResolved || won.Answer != "yes" {
		t.Fatalf("claimed ask should be resolved with answer, got status=%q answer=%q", won.Status, won.Answer)
	}

	// Second concurrent answer must lose (no longer waiting).
	if _, err := d.ClaimSessionAsk(ctx, a.ID, "no"); err == nil {
		t.Fatal("second claim should fail (already resolved)")
	}
}

// TestListWaitingSessionAsks_ExcludesTerminal verifies the sweeper/boot-restore
// list only surfaces still-waiting asks: a resolved one drops out, and CloseSessionAsk
// on an already-resolved ask is a no-op that reports the lost race.
func TestListWaitingSessionAsks_ExcludesTerminal(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := d.CreateSessionAsk(ctx, SessionAsk{SessionID: "SES1", State: "{}"})
	gone, _ := d.CreateSessionAsk(ctx, SessionAsk{SessionID: "SES2", State: "{}"})
	if _, err := d.ClaimSessionAsk(ctx, gone.ID, "x"); err != nil {
		t.Fatal(err)
	}

	waiting, err := d.ListWaitingSessionAsks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 1 || waiting[0].ID != keep.ID {
		t.Fatalf("only the still-waiting ask should list, got %+v", waiting)
	}

	// Closing an already-resolved ask loses the race → false, no status change.
	closed, err := d.CloseSessionAsk(ctx, gone.ID, SessionAskCancelled)
	if err != nil {
		t.Fatal(err)
	}
	if closed {
		t.Fatal("closing a resolved ask should report the lost race (false)")
	}

	// Timeout-closing a waiting ask succeeds and drops it from the waiting list.
	closed, err = d.CloseSessionAsk(ctx, keep.ID, SessionAskTimeout)
	if err != nil || !closed {
		t.Fatalf("closing a waiting ask should succeed, got closed=%v err=%v", closed, err)
	}
	waiting, _ = d.ListWaitingSessionAsks(ctx)
	if len(waiting) != 0 {
		t.Fatalf("no asks should remain waiting, got %+v", waiting)
	}
}

// TestSessionAsk_PersistsAcrossReopen verifies a waiting ask survives a store
// reopen (the restart-durability the feature exists for).
func TestSessionAsk_PersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := d.CreateSessionAsk(ctx, SessionAsk{SessionID: "SES1", CallID: "c1", State: `{"x":1}`})
	if err != nil {
		t.Fatal(err)
	}

	d2, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.GetSessionAsk(ctx, a.ID)
	if err != nil {
		t.Fatalf("ask should survive reopen: %v", err)
	}
	if got.Status != SessionAskWaiting || got.CallID != "c1" || got.State != `{"x":1}` {
		t.Fatalf("reopened ask mismatch: %+v", got)
	}
}
