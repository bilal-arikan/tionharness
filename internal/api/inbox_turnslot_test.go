package api

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/config"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/settings"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// newWorkspaceServer builds a real (if minimal) Server over a temp workspace tree:
// the send-queue tests below need a live Runtime, because the property under test is
// exactly how the queue and the runtime's turn slot interact.
// Takes testing.TB rather than *testing.T so benchmarks can use the same harness
// (BenchmarkWorkspaceRunning needs a real store + Runtime, not a stub).
func newWorkspaceServer(t testing.TB) (*Server, *workspace.Workspace) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := t.TempDir()
	registry := providers.NewRegistry("")
	tun := agent.NewTunables()
	cipher, err := config.LoadSecret(dir)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	bus := events.NewBus()
	logs := logbuf.New(64)
	manager, err := workspace.NewManager(dir, registry, tun, cipher, bus, logs, logger)
	if err != nil {
		t.Fatalf("workspace manager: %v", err)
	}
	t.Cleanup(manager.Close)
	settingsStore, err := settings.Open(dir, cipher)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	s := NewServer(manager, registry, settingsStore, tun, logs, bus, logger)
	// A fresh tree has NO workspace (first-run onboarding owns creation), so make one.
	wsp, err := manager.Create("test", "", "test")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return s, wsp
}

// TestQueuedMessageStaysWaitingWhileTurnSlotHeld is the regression guard for the
// bug this whole seam exists for (_Docs/58): a message queued while the session's
// runtime turn slot is taken — a coordinator mid-drain, a worker notification turn,
// a scheduler wake — must stay WAITING in the queue, visible and cancellable, until
// the slot frees. The old worker popped it into the in-flight slot FIRST and then
// blocked inside the turn, so it vanished from every window's queue tray and only
// reappeared (as a chat bubble) whenever the coordinator finally released the slot.
func TestQueuedMessageStaysWaitingWhileTurnSlotHeld(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Something else owns the session's turn slot (stand-in for a coordinator turn
	// triggered by a worker notification).
	releaseSlot := wsp.Runtime.BeginSessionUserTurn(sess.ID)

	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: "merhaba"}, "m1") {
		t.Fatal("enqueue rejected")
	}

	// Give the worker every chance to (wrongly) dispatch.
	time.Sleep(150 * time.Millisecond)
	s.inbox.lock()
	ib := s.inbox.at(wsp.ID, sess.ID)
	waiting := len(ib.items)
	inflight := ib.inflight
	s.inbox.unlock()
	if waiting != 1 {
		t.Fatalf("queued message must stay WAITING while the turn slot is held, got %d waiting", waiting)
	}
	if inflight != nil {
		t.Fatal("queued message must not be popped into the in-flight slot before the turn slot is free")
	}

	// It is still cancellable — the whole point of leaving it in the queue.
	if !s.cancelQueued(wsp.ID, sess.ID, "m1") {
		t.Fatal("a message waiting on the turn slot must still be cancellable")
	}
	releaseSlot()
}

// TestQueuedMessageDispatchesAfterTurnSlotReleased: the wait is not a wedge — once
// the slot frees, the worker picks the message up on its own.
func TestQueuedMessageDispatchesAfterTurnSlotReleased(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	releaseSlot := wsp.Runtime.BeginSessionUserTurn(sess.ID)
	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: "merhaba"}, "m1") {
		t.Fatal("enqueue rejected")
	}
	time.Sleep(50 * time.Millisecond)
	releaseSlot()

	deadline := time.After(3 * time.Second)
	for {
		s.inbox.lock()
		ib := s.inbox.at(wsp.ID, sess.ID)
		waiting := len(ib.items)
		s.inbox.unlock()
		if waiting == 0 {
			return // dispatched (the turn itself fails: no provider is configured)
		}
		select {
		case <-deadline:
			t.Fatal("message never dispatched after the turn slot was released")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
