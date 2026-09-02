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

// TestLivenessComposesSources: the snapshot folds the turn slots, the api probe,
// durable asks, waiting flow runs and coordinator slots, and reports capacity.
func TestLivenessComposesSources(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	busy, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "busy"})
	asked, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "asked"})
	flowSess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "flow", SourceID: "FLW1"})
	idle, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "idle"})
	chat, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "chat"})

	// A turn holding the admission slot.
	release := rt.claimSessionTurnSlot(busy.ID, turnqueue.KindWorker, "task")
	defer release()
	// The api's streamed chat run, seen through the probe.
	rt.SetExternalActiveSessions(func() []string { return []string{chat.ID} })
	// A parked durable ask.
	if _, err := rt.db.CreateSessionAsk(ctx, db.SessionAsk{SessionID: asked.ID, AgentID: a.ID, Status: db.SessionAskWaiting, Kind: "ask", CallID: "c1", Payload: `{"question":"ok?"}`}); err != nil {
		t.Fatalf("create ask: %v", err)
	}
	// A flow run suspended at await-input, owned by flowSess.
	flow, _ := rt.db.CreateFlow(ctx, db.Flow{Name: "f", Graph: `{"start":"s","nodes":[{"id":"s","type":"start"}]}`})
	run, _ := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID, SessionID: flowSess.ID})
	if err := rt.db.MarkFlowRunWaiting(ctx, run.ID, `{}`); err != nil {
		t.Fatalf("mark waiting: %v", err)
	}
	// A coordinator with one live worker and an owed drain turn.
	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)
	slot.workers.Add(1)
	slot.mu.Lock()
	slot.workerPending = true
	slot.mu.Unlock()
	t.Cleanup(func() {
		slot.workers.Add(-1)
		slot.mu.Lock()
		slot.workerPending = false
		slot.mu.Unlock()
	})

	snap := rt.Liveness(ctx)
	want := map[string]liveness.State{
		busy.ID:     liveness.Running,
		chat.ID:     liveness.Running,
		asked.ID:    liveness.WaitingAsk,
		flowSess.ID: liveness.WaitingInput,
		coord:       liveness.Queued, // an owed drain beats "awaiting workers"
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
	if e, _ := snap.Get(busy.ID); e.Reason != "turn:worker" || e.Since == 0 {
		t.Errorf("busy entry = %+v, want reason turn:worker with a start time", e)
	}
	if e, _ := snap.Get(chat.ID); e.Reason != "turn:chat" {
		t.Errorf("chat entry = %+v, want reason turn:chat", e)
	}
	set := snap.RunningSet()
	if !set[busy.ID] || !set[chat.ID] || set[asked.ID] || set[flowSess.ID] || set[coord] {
		t.Errorf("RunningSet = %v", set)
	}
	if snap.Capacity.SpawnMax != tun.SpawnMaxConcurrent() || snap.Capacity.QueueMax != tun.SpawnQueueMax() || snap.Capacity.BusyTurns != 1 {
		t.Errorf("capacity = %+v", snap.Capacity)
	}

	// Once the drain is no longer owed, the live worker makes it AwaitingWorkers;
	// a coordinator running its own turn is Running instead.
	slot.mu.Lock()
	slot.workerPending = false
	slot.mu.Unlock()
	if snap := rt.Liveness(ctx); !snap.Is(coord, liveness.AwaitingWorkers) || !snap.RunningSet()[coord] {
		t.Errorf("coordinator with live worker: %+v", snap.Entries)
	}
	rel := rt.claimSessionTurnSlot(coord, turnqueue.KindCoordinator, "drain")
	if snap := rt.Liveness(ctx); !snap.Is(coord, liveness.Running) {
		t.Errorf("coordinator running its own turn: %+v", snap.Entries)
	}
	rel()
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
