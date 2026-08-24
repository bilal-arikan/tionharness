package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// drainEvents non-blockingly collects every event currently buffered on ch.
// Bus.Publish is synchronous, so anything published before the call is present.
func drainEvents(ch <-chan events.Event) []events.Event {
	var out []events.Event
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

// TestNotifyNewAnomalies verifies the anomaly→notification bridge: a warn-level
// debug anomaly raises exactly one desktop-notification event (deep-linked to
// the session), info-level findings are NOT toasted, and a persistent anomaly is
// deduped so it notifies once, not on every turn.
func TestNotifyNewAnomalies(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	bus := events.NewBus()
	rt.bus = bus
	id, ch := bus.Subscribe()
	defer bus.Unsubscribe(id)

	ctx := context.Background()
	sess, _ := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: "AGT1"})

	// 3 calls, big prompt spend, almost no cache reads → warn low_cache_hit.
	// The majority-thinking output also raises info high_thinking, which must be
	// excluded from toasts.
	for i := 0; i < 3; i++ {
		_ = rt.db.AppendDebugEvent(sess.ID, db.DebugEvent{
			Type: db.DebugLLMCall, Model: "m", In: 10000, Out: 3000, Think: 2000, CacheRead: 500,
		}, 0)
	}

	rt.notifyNewAnomalies(ctx, sess.ID)
	got := drainEvents(ch)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 warn anomaly toast (info excluded), got %d: %+v", len(got), got)
	}
	if got[0].Type != "anomaly" || got[0].Target["view"] != "chat" || got[0].Target["sessionId"] != sess.ID {
		t.Fatalf("bad event: type=%q target=%+v", got[0].Type, got[0].Target)
	}

	// Second pass over the same still-standing anomaly → no re-notification.
	rt.notifyNewAnomalies(ctx, sess.ID)
	if again := drainEvents(ch); len(again) != 0 {
		t.Fatalf("dedup failed: re-notified %d events", len(again))
	}
}
