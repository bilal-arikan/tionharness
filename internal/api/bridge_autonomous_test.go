package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
)

// waitForStep publishes a session_step bus event repeatedly (tolerating the
// bridge's subscribe race) and reports whether a KindStep frame reaches the hub
// within the deadline.
func waitForStep(bus *events.Bus, hubCh <-chan sessionhub.Event, sid string) bool {
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	pub := func() {
		bus.Publish(events.Event{
			Type:   "session_step",
			Target: map[string]string{"sessionId": sid},
			Step:   json.RawMessage(`{"kind":"tool","text":"ran a tool"}`),
		})
	}
	pub()
	for {
		select {
		case ev := <-hubCh:
			if ev.Kind == sessionhub.KindStep {
				return true
			}
		case <-tick.C:
			pub() // re-publish until the bridge goroutine has subscribed
		case <-deadline:
			return false
		}
	}
}

// TestBridgeForwardsAutonomousSteps verifies that an AUTONOMOUS run's live steps
// are bridged to the hub. The token-only handle autonomousInteraction registers
// must NOT suppress bridging — otherwise a window watching a coordinator/scheduler
// turn sees nothing until the turn ends (the reported bug).
func TestBridgeForwardsAutonomousSteps(t *testing.T) {
	bus := events.NewBus()
	srv := &Server{
		bus:  bus,
		hub:  sessionhub.New("test", 0),
		runs: newChatRuns(),
	}
	const sid = "s-auto"
	// An autonomous, token-only run owns the session (mirrors autonomousInteraction).
	run := srv.runs.register("r-auto", sid, "", func() {})
	run.autonomous = true
	defer srv.runs.unregister("r-auto")

	_, ch, _ := srv.hub.Subscribe(sid)
	go srv.bridgeBusToHub()

	if !waitForStep(bus, ch, sid) {
		t.Fatal("autonomous session_step was not bridged to the hub")
	}
}

// TestBridgeSkipsInteractiveSteps verifies the double-publish guard still holds:
// an INTERACTIVE run self-publishes to the hub (runChatTurn onStep), so the bridge
// must NOT also forward its bus copy.
func TestBridgeSkipsInteractiveSteps(t *testing.T) {
	bus := events.NewBus()
	srv := &Server{
		bus:  bus,
		hub:  sessionhub.New("test", 0),
		runs: newChatRuns(),
	}
	const sid = "s-inter"
	run := srv.runs.register("r-inter", sid, "", func() {})
	run.autonomous = false // interactive turn: it self-publishes steps
	defer srv.runs.unregister("r-inter")

	_, ch, _ := srv.hub.Subscribe(sid)
	go srv.bridgeBusToHub()

	if waitForStep(bus, ch, sid) {
		t.Fatal("interactive session_step was double-published by the bridge")
	}
}
