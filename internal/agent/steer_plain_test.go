package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestPlainTurnFoldsPendingSteer is the regression guard for the silently lost
// steer on the NO-TOOLS path: a turn with MCP off never enters the native tool
// loop, which used to be the only place drainSteer ran, so guidance sent before
// the single completion evaporated. It must reach the provider request as live
// user guidance and be recorded as a step, exactly like the tool-loop path.
func TestPlainTurnFoldsPendingSteer(t *testing.T) {
	rt := loopRuntime(t)
	steer := make(chan string, 4)
	steer <- "stop and check the migration first"
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	fp := &fakeProvider{script: []scriptedResp{{stop: providers.StopEndTurn, text: "ok"}}}
	// MCPEnabled false → completeTracedInner takes the plain path (runPlain).
	agent := db.Agent{ID: "a1", Model: "m"}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "start"}}}

	_, steps, err := rt.CompleteWithToolsTraced(ctx, agent, fp, req, false)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (plain path)", fp.calls)
	}
	sent := fp.requests[0].Messages
	last := sent[len(sent)-1]
	if !strings.Contains(last.Text, "stop and check the migration first") {
		t.Fatalf("steer never reached the provider request; last message = %#v", last)
	}
	if !strings.HasPrefix(last.Text, steerPrefix) {
		t.Fatalf("steer injected without its live-guidance prefix: %q", last.Text)
	}
	if !hasSteerStep(steps, "stop and check the migration first") {
		t.Fatalf("plain turn produced no steer step: %#v", steps)
	}
	if len(steer) != 0 {
		t.Fatal("steer channel must be drained by the plain path")
	}
}

// TestPlainTurnLeavesLateSteerOnChannel pins the other half of the contract: a
// message that arrives too late for the single completion stays ON the channel
// so the caller's turn-end fallback can requeue it. Consuming it here without
// injecting it anywhere would be the same silent loss in a new place.
func TestPlainTurnLeavesLateSteerOnChannel(t *testing.T) {
	rt := loopRuntime(t)
	steer := make(chan string, 4)
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	fp := &fakeProvider{onRequest: func(int, providers.Request) {
		steer <- "too late for this completion"
	}}
	agent := db.Agent{ID: "a1", Model: "m"}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "start"}}}

	if _, _, err := rt.CompleteWithToolsTraced(ctx, agent, fp, req, false); err != nil {
		t.Fatalf("complete: %v", err)
	}
	select {
	case got := <-steer:
		if got != "too late for this completion" {
			t.Fatalf("steer channel holds %q", got)
		}
	default:
		t.Fatal("a steer that arrived mid-completion must stay queued for the turn-end fallback")
	}
}

func hasSteerStep(steps []TurnStep, text string) bool {
	for _, s := range steps {
		if s.Kind == StepSteer && s.Text == text {
			return true
		}
	}
	return false
}
