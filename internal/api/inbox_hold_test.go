package api

import (
	"context"
	"testing"
	"time"
)

// TestAcquireInboxSlot_BlocksDispatchWhileHeld: a slash command (e.g. /compact)
// that runs outside runInboxWorker claims the serial slot so a message submitted
// mid-op waits in the tray instead of being dispatched concurrently. While held,
// kickInbox must NOT start a worker even though the queue is non-empty; release
// clears the hold.
func TestAcquireInboxSlot_BlocksDispatchWhileHeld(t *testing.T) {
	s := &Server{inbox: newInboxStore()}

	release, err := s.acquireInboxSlot(context.Background(), "SES", "")
	if err != nil {
		t.Fatalf("acquire on an idle session should not error: %v", err)
	}

	// A message arrives during the op: enqueue it directly (no workspaces in this
	// unit test) and try to kick the worker — the hold must keep it waiting.
	s.inbox.lock()
	s.inbox.sessions["SES"].items = append(s.inbox.sessions["SES"].items, inboxItem{ClientMsgID: "m1"})
	s.inbox.unlock()

	s.kickInbox("SES")
	s.inbox.lock()
	waiting := len(s.inbox.sessions["SES"].items)
	s.inbox.unlock()
	if waiting != 1 {
		t.Fatalf("held slot must keep the message waiting, got %d items", waiting)
	}

	// Drop the item so release's drain-kick is a no-op (no workspace to dispatch to),
	// then assert release clears the running flag.
	s.inbox.lock()
	s.inbox.sessions["SES"].items = nil
	s.inbox.unlock()
	release()
	s.inbox.lock()
	running := s.inbox.sessions["SES"].running
	s.inbox.unlock()
	if running {
		t.Fatal("release must clear the hold (running flag)")
	}
}

// TestAcquireInboxSlot_WaitsForRunningWorker: when a real queued turn is already
// draining, acquire must BLOCK (not race) until the slot frees, then claim it. It
// wakes on the idle broadcast, not a poll.
func TestAcquireInboxSlot_WaitsForRunningWorker(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions["SES"] = &sessionInbox{seen: map[string]bool{}, running: true}

	acquired := make(chan func(), 1)
	go func() {
		release, err := s.acquireInboxSlot(context.Background(), "SES", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			acquired <- func() {}
			return
		}
		acquired <- release
	}()

	// The acquirer must still be blocked while the worker "runs".
	select {
	case <-acquired:
		t.Fatal("acquire returned while a worker was still running")
	case <-time.After(50 * time.Millisecond):
	}

	// Worker finishes its drain: clear running and broadcast idle.
	s.inbox.lock()
	s.inbox.sessions["SES"].running = false
	signalInboxIdleLocked(s.inbox.sessions["SES"])
	s.inbox.unlock()

	select {
	case release := <-acquired:
		s.inbox.lock()
		running := s.inbox.sessions["SES"].running
		s.inbox.unlock()
		if !running {
			t.Fatal("acquire must claim the slot (running = true) once free")
		}
		release()
	case <-time.After(2 * time.Second):
		t.Fatal("acquire did not wake after the slot went idle")
	}
}

// TestAcquireInboxSlot_CtxCancelBails: a client disconnect / timeout while waiting
// for a busy slot returns an error and a no-op release, without ever claiming the
// slot (so the running worker is left untouched).
func TestAcquireInboxSlot_CtxCancelBails(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions["SES"] = &sessionInbox{seen: map[string]bool{}, running: true}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		release, err := s.acquireInboxSlot(ctx, "SES", "")
		release() // must be a safe no-op even on error
		done <- err
	}()

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a context error when the wait is abandoned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("acquire did not return after ctx cancel")
	}
	s.inbox.lock()
	running := s.inbox.sessions["SES"].running
	s.inbox.unlock()
	if !running {
		t.Fatal("a cancelled acquire must leave the running worker untouched")
	}
}
