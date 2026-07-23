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
// within the deadline. origin tags the event's source ("interactive" or "" for
// autonomous), exactly as emitSessionStep does.
func waitForStep(bus *events.Bus, hubCh <-chan sessionhub.Event, sid, origin string) bool {
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	pub := func() {
		target := map[string]string{"sessionId": sid}
		if origin != "" {
			target["origin"] = origin
		}
		bus.Publish(events.Event{
			Type:   "session_step",
			Target: target,
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

// TestBridgeForwardsAutonomousSteps verifies that an AUTONOMOUS turn's live steps
// (origin unset) are bridged to the hub. These turns do NOT self-publish to the
// hub, so without the bridge a window watching a coordinator/scheduler/spawn/wake
// turn would see nothing until the turn ends (the reported bug).
func TestBridgeForwardsAutonomousSteps(t *testing.T) {
	bus := events.NewBus()
	srv := &Server{
		bus:  bus,
		hub:  sessionhub.New("test", 0),
		runs: newChatRuns(),
	}
	const sid = "s-auto"
	_, ch, _ := srv.hub.Subscribe(sid)
	go srv.bridgeBusToHub()

	if !waitForStep(bus, ch, sid, "") {
		t.Fatal("autonomous session_step was not bridged to the hub")
	}
}

// TestBridgeSkipsInteractiveSteps verifies the double-publish guard: an
// INTERACTIVE turn self-publishes its steps to the hub (runChatTurn onStep), so
// its mirrored bus copy — tagged origin=interactive — must NOT be re-forwarded.
// The skip is origin-driven, NOT run-liveness-driven: no run is registered here,
// mirroring a STRAGGLER bus step processed AFTER the interactive run already
// unregistered (defer). Re-bridging that straggler would re-arm the client's live
// "conversing" indicator that turn_done just cleared — the stuck-live bug.
func TestBridgeSkipsInteractiveSteps(t *testing.T) {
	bus := events.NewBus()
	srv := &Server{
		bus:  bus,
		hub:  sessionhub.New("test", 0),
		runs: newChatRuns(),
	}
	const sid = "s-inter"
	_, ch, _ := srv.hub.Subscribe(sid)
	go srv.bridgeBusToHub()

	if waitForStep(bus, ch, sid, "interactive") {
		t.Fatal("interactive session_step was double-published by the bridge")
	}
}
