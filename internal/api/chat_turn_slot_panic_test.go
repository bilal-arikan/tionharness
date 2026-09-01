package api

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestPreflightPanicReleasesTurnSlot guards the window between the turn-slot claim
// inside preflight and preflight returning it to runChatTurn, which arms the
// `defer release()`. A panic in that window used to leak the per-session slot
// forever: runTurnGuarded recovers the panic, so the process survives and every
// later turn on that session blocks on a slot nobody will ever release.
//
// The panic is injected without any production test hook: the turn is built with a
// nil run, and a session with no agent makes preflight take its agent_not_found
// path — which reports through t.run.emit and nil-derefs, right inside the window.
func TestPreflightPanicReleasesTurnSlot(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	turn := &chatTurn{
		s:        s,
		wsp:      wsp,
		database: wsp.DB,
		req:      chatReq{SessionID: sess.ID, Message: "merhaba"},
		ctx:      ctx,
	}
	panicked := func() (p bool) {
		defer func() { p = recover() != nil }()
		turn.preflight()
		return
	}()
	// The panic must still propagate: swallowing it here would hide the real bug
	// from runTurnGuarded and from the failure report.
	if !panicked {
		t.Fatal("preflight must still propagate the panic to its caller")
	}

	// The slot must be free again. ClaimSessionUserTurn is the ctx-aware form, so a
	// leaked slot shows up as a deadline error instead of hanging the test forever.
	claimCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release, cerr := wsp.Runtime.ClaimSessionUserTurn(claimCtx, sess.ID)
	if cerr != nil {
		t.Fatalf("turn slot leaked: a panic in preflight left the session's slot held: %v", cerr)
	}
	release()
}

// TestPreflightReleasesTurnSlotOnceOnTheNormalPath: the panic guard must not
// double-release. On the normal (non-panic) failure path preflight hands the
// release func back and releases nothing itself; the slot stays held until the
// caller's defer runs, exactly once.
func TestPreflightReleasesTurnSlotOnceOnTheNormalPath(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	run := s.runs.register("run-1", sess.ID, wsp.ID, func() {})
	defer s.runs.unregister("run-1")
	turn := &chatTurn{
		s:        s,
		wsp:      wsp,
		database: wsp.DB,
		req:      chatReq{SessionID: sess.ID, Message: "merhaba"},
		run:      run,
		sse:      run.emit,
		ctx:      ctx,
	}
	// No agent on the session → agent_not_found, the first post-claim failure path.
	release, ok := turn.preflight()
	if ok {
		t.Fatal("preflight must fail when the session has no agent")
	}
	if release == nil {
		t.Fatal("preflight must hand the claimed slot's release back to the caller")
	}

	// Still held: preflight returned normally, so its panic guard released nothing.
	heldCtx, cancelHeld := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelHeld()
	if rel, cerr := wsp.Runtime.ClaimSessionUserTurn(heldCtx, sess.ID); cerr == nil {
		rel()
		t.Fatal("the slot must still be held when preflight returns it to the caller")
	}

	release() // the caller's defer — the single release
	claimCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rel, cerr := wsp.Runtime.ClaimSessionUserTurn(claimCtx, sess.ID)
	if cerr != nil {
		t.Fatalf("the caller's release must free the slot: %v", cerr)
	}
	rel()
}
