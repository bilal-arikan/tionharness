package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/liveness"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// TestLivenessComposesSources: the snapshot folds the turn slots, the api probe
// and durable asks, and reports capacity.
func TestLivenessComposesSources(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	busy, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "busy"})
	asked, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "asked"})
	idle, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "idle"})
	chat, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "chat"})

	// A turn holding the admission slot.
	release := rt.claimSessionTurnSlot(busy.ID, turnqueue.KindSpawn, "task")
	defer release()
	// The api's streamed chat run, seen through the probe.
	rt.SetExternalActiveSessions(func() []string { return []string{chat.ID} })
	// A parked durable ask.
	if _, err := rt.db.CreateSessionAsk(ctx, db.SessionAsk{SessionID: asked.ID, AgentID: a.ID, Status: db.SessionAskWaiting, Kind: "ask", CallID: "c1", Payload: `{"question":"ok?"}`}); err != nil {
		t.Fatalf("create ask: %v", err)
	}
	snap := rt.Liveness(ctx)
	want := map[string]liveness.State{
		busy.ID:  liveness.Running,
		chat.ID:  liveness.Running,
		asked.ID: liveness.WaitingAsk,
	}
	for sid, st := range want {
		if !snap.Is(sid, st) {
			e, _ := snap.Get(sid)
			t.Errorf("%s: state = %+v, want %s", sid, e, st)
		}
	}
	if _, ok := snap.Get(idle.ID); ok {
		t.Errorf("idle session must not appear: %+v", snap.Entries)
	}
	if e, _ := snap.Get(busy.ID); e.Reason != "turn:spawn" || e.Since == 0 {
		t.Errorf("busy entry = %+v, want reason turn:spawn with a start time", e)
	}
	if e, _ := snap.Get(chat.ID); e.Reason != "turn:chat" {
		t.Errorf("chat entry = %+v, want reason turn:chat", e)
	}
	set := snap.RunningSet()
	if !set[busy.ID] || !set[chat.ID] || set[asked.ID] {
		t.Errorf("RunningSet = %v", set)
	}
	if snap.Capacity.SpawnMax != tun.SpawnMaxConcurrent() || snap.Capacity.QueueMax != tun.SpawnQueueMax() || snap.Capacity.BusyTurns != 1 {
		t.Errorf("capacity = %+v", snap.Capacity)
	}

	rt.SetPaused(true)
	if snap := rt.Liveness(ctx); !snap.Capacity.AutonomyPaused {
		t.Error("capacity must report the autonomy brake")
	}
}

// TestTurnSlotEmitsLivenessEvent: claiming and releasing a slot publishes
// ws:liveness with busy true then false.
func TestTurnSlotEmitsLivenessEvent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	release := rt.claimSessionTurnSlot("SES-live", turnqueue.KindUser, "")
	release()
	var seen []LivenessPayload
	for _, e := range drain() {
		if e.Type == events.TypeWSLiveness {
			seen = append(seen, decodeData[LivenessPayload](t, e))
		}
	}
	if len(seen) != 2 || !seen[0].Busy || seen[0].Kind != string(turnqueue.KindUser) || seen[1].Busy {
		t.Fatalf("liveness events = %+v, want busy(user) then idle", seen)
	}
}
