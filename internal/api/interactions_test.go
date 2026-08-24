package api

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

// Concurrent answers to one interaction must resolve exactly once (first-writer-
// wins CAS): every other window's answer loses and the blocked tool call receives
// the single winning reply.
func TestResolveInteractionCAS(t *testing.T) {
	srv := &Server{hub: sessionhub.New("t", 0), interactions: newInteractionStore()}
	pi := srv.openInteraction("ws1", "s1", "ask", map[string]any{"question": "q"})

	const N = 16
	var wins int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // maximise the race
			if srv.resolveInteraction("ws1", "s1", pi.id, fmt.Sprintf("ans-%d", i), "w") {
				atomic.AddInt64(&wins, 1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("want exactly 1 CAS winner, got %d", wins)
	}
	// The winning answer was delivered exactly once to the blocked tool call.
	select {
	case ans := <-pi.answer:
		if ans == "" {
			t.Fatal("winning answer is empty")
		}
	default:
		t.Fatal("winning answer not delivered")
	}
	// The interaction is gone from the store (removed on resolve).
	if got := srv.interactions.get("ws1", "s1", pi.id); got != nil {
		t.Fatal("resolved interaction should be removed from the store")
	}
}
