package agent

import (
	"context"
	"testing"
)

// An autonomous turn (schedule / wake / spawn / coordination) registers its own
// cancel func via trackSession; CancelSession must cancel exactly that context so
// the API's "stop" action can reach runs that never enter the chatRuns registry.
func TestCancelSessionCancelsTrackedTurn(t *testing.T) {
	rt := &Runtime{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.trackSession("sess-1", cancel)

	if !rt.CancelSession("sess-1") {
		t.Fatal("CancelSession returned false for a tracked session")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("tracked turn context was not cancelled")
	}

	rt.untrackSession("sess-1")
	if rt.CancelSession("sess-1") {
		t.Fatal("CancelSession returned true after untrackSession")
	}
	if rt.CancelSession("unknown") {
		t.Fatal("CancelSession returned true for an unknown session")
	}
}
