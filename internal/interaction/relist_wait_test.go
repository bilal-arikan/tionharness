package interaction

import (
	"testing"
	"time"
)

// TestPushToolsChangedAndWaitSignalled: with an open stream, the wait returns true as
// soon as a tools/list re-fetch signals the waiter (the good path the live probe saw).
func TestPushToolsChangedAndWaitSignalled(t *testing.T) {
	srv := NewServer(&fakeBackend{validToken: "tok"}, nil)
	key := streamKey("tok", "extended")
	srv.mu.Lock()
	srv.streams[key] = make(chan []byte, 1) // pretend the CLI has an open SSE stream
	srv.mu.Unlock()

	done := make(chan bool, 1)
	go func() { done <- srv.PushToolsChangedAndWait("tok", 2*time.Second) }()

	time.Sleep(20 * time.Millisecond)
	srv.signalRelist(key) // the tools/list handler would call this after re-listing

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("wait must return true when the re-list is signalled")
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return after signalRelist")
	}
}

// TestPushToolsChangedAndWaitNoStream: with no open stream the push reaches nobody, so
// the wait must return false IMMEDIATELY (never block — nothing would ever signal it).
func TestPushToolsChangedAndWaitNoStream(t *testing.T) {
	srv := NewServer(&fakeBackend{validToken: "tok"}, nil)
	start := time.Now()
	if srv.PushToolsChangedAndWait("tok", 2*time.Second) {
		t.Fatal("expected false with no open stream")
	}
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("must not block without a stream, waited %v", d)
	}
}

// TestPushToolsChangedAndWaitTimeout: a stream exists but the client never re-lists →
// the wait honors the timeout and returns false (bounded; can never wedge activate).
func TestPushToolsChangedAndWaitTimeout(t *testing.T) {
	srv := NewServer(&fakeBackend{validToken: "tok"}, nil)
	key := streamKey("tok", "extended")
	srv.mu.Lock()
	srv.streams[key] = make(chan []byte, 1)
	srv.mu.Unlock()

	start := time.Now()
	if srv.PushToolsChangedAndWait("tok", 150*time.Millisecond) {
		t.Fatal("expected false when no re-list arrives")
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Fatalf("should have waited the full timeout, waited %v", d)
	}
	// The timed-out waiter must be cleaned up so a later re-list finds nothing.
	srv.mu.Lock()
	n := len(srv.relistWaiters[key])
	srv.mu.Unlock()
	if n != 0 {
		t.Fatalf("timed-out waiter not cleaned up: %d left", n)
	}
}
