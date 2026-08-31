package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// stalledStreamKind is the kind id the fake transport below registers itself as.
// It is deliberately namespaced so it can never collide with a real transport if
// the kind registry is ever walked by another test in this package.
const stalledStreamKind = "test-stalled-stream"

// stalledStreamProvider models the exact failure the inactivity watchdog exists
// for: the provider's stream is OPEN but silent — no token, no thinking chunk,
// no tool delta, ever. It implements Streamer on purpose. The non-streaming
// completion path is heartbeated (recordedComplete) and must NOT be reclaimed on
// idle; only a stalled STREAM is, so the streaming path is the one under test.
//
// Every call blocks until the turn's context is cancelled and reports the
// cancellation cause, which is how a real streaming client surfaces a turn that
// was cut out from under it.
type stalledStreamProvider struct {
	mu sync.Mutex
	// prompts is the set of last-user-message texts the transport was entered
	// with. A set rather than a counter because a reclaimed turn may legitimately
	// be retried by the runtime's provider-retry loop — what the queue property
	// needs is that BOTH queued messages reached the provider, not how many
	// attempts each took.
	prompts map[string]bool
}

func (p *stalledStreamProvider) Name() string { return stalledStreamKind }

func (p *stalledStreamProvider) Complete(ctx context.Context, req providers.Request) (*providers.Response, error) {
	return p.Stream(ctx, req, nil)
}

func (p *stalledStreamProvider) Stream(ctx context.Context, req providers.Request, onDelta func(providers.StreamDelta)) (*providers.Response, error) {
	p.mu.Lock()
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == providers.RoleUser {
			p.prompts[req.Messages[i].Text] = true
			break
		}
	}
	p.mu.Unlock()
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

// sawPrompt reports whether a turn carrying text as its last user message ever
// reached the transport.
func (p *stalledStreamProvider) sawPrompt(text string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prompts[text]
}

// useStalledProvider registers the fake transport, points the registry at a
// single instance of it, and returns the provider so the test can count how many
// turns actually reached it. Registration is idempotent (the kind registry is a
// map keyed by kind id) and this package runs its tests sequentially, so the
// global write is safe here.
func useStalledProvider(t *testing.T, s *Server) *stalledStreamProvider {
	t.Helper()
	prov := &stalledStreamProvider{prompts: map[string]bool{}}
	providers.RegisterKind(providers.NewBuiltinKind(
		providers.Manifest{
			Kind:      stalledStreamKind,
			Label:     "Stalled stream (test)",
			Order:     9000,
			Transport: "api",
		},
		func(providers.ResolvedConfig) bool { return true },
		func(providers.ResolvedConfig) (providers.Provider, error) { return prov, nil },
	))
	s.providers.SetInstances([]providers.Instance{{
		ID:      stalledStreamKind,
		KindID:  stalledStreamKind,
		Label:   "Stalled stream (test)",
		Enabled: true,
	}})
	return prov
}

// TestInboxQueueDrainsAfterStalledTurnIsReclaimed is the end-to-end guard for the
// bug this card is about: the serial inbox worker runs one turn at a time and
// blocks inside runTurnGuarded until the turn RETURNS, so a turn whose provider
// stream dies silently used to pin the worker forever and every message queued
// behind it was never delivered.
//
// The chain under test, in order: the inactivity watchdog cuts the stalled turn
// with ErrTurnIdleTimeout → runChatTurn takes the failure path, persists what it
// had and returns → the worker's lap ends → the SECOND queued message is
// dispatched. Asserting only "the first turn errored" would not catch a
// regression that leaves the worker wedged, so the load-bearing assertion is
// that the provider was entered a SECOND time.
func TestInboxQueueDrainsAfterStalledTurnIsReclaimed(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	// Sub-minute inactivity window: the production tunable's smallest value is one
	// minute, which is not a test.
	s.chatTurnIdleOverride = 300 * time.Millisecond
	prov := useStalledProvider(t, s)

	ctx := context.Background()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{
		Name:     "stalled",
		Provider: stalledStreamKind,
		Model:    "test-model",
		// No tools: the tool-enabled path routes through the heartbeated
		// completion call, and it is the bare streaming path that the inactivity
		// watchdog governs.
		MCPEnabled: false,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: "birinci"}, "m1") {
		t.Fatal("first enqueue rejected")
	}
	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: "ikinci"}, "m2") {
		t.Fatal("second enqueue rejected")
	}

	// Both turns stall and are reclaimed one after the other, so the queue needs
	// roughly two idle windows to drain; the deadline is generous against that.
	deadline := time.After(20 * time.Second)
	for {
		s.inbox.lock()
		ib := s.inbox.at(wsp.ID, sess.ID)
		waiting, inflight, running := 0, (*inboxItem)(nil), false
		if ib != nil {
			waiting, inflight, running = len(ib.items), ib.inflight, ib.running
		}
		s.inbox.unlock()
		if waiting == 0 && inflight == nil && !running {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("queue never drained after the stalled turn: waiting=%d inflight=%v running=%v",
				waiting, inflight != nil, running)
		case <-time.After(20 * time.Millisecond):
		}
	}

	if !prov.sawPrompt("birinci") {
		t.Fatal("the first queued message never reached the provider")
	}
	if !prov.sawPrompt("ikinci") {
		t.Fatal("the message queued behind the stalled turn was never delivered to the provider")
	}

	// The stalled turn must leave a durable trace of WHY it ended, not a silent
	// gap: both user messages and an idle-timeout error card per turn.
	msgs, err := wsp.DB.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	users, idleErrors := 0, 0
	for _, m := range msgs {
		if m.Text == "birinci" || m.Text == "ikinci" {
			users++
		}
		for _, st := range unmarshalStepsForTest(t, m.Steps) {
			if st.Reason == reasonTurnIdleTimeout {
				idleErrors++
			}
		}
	}
	if users != 2 {
		t.Fatalf("persisted %d user messages, want 2", users)
	}
	if idleErrors != 2 {
		t.Fatalf("persisted %d idle-timeout error steps, want 2 (one per reclaimed turn)", idleErrors)
	}

	// No stranded crash-recovery sidecar: a turn the watchdog reclaimed returned
	// through a normal path, so the next boot must not re-dispatch it.
	if _, ok, err := wsp.DB.ReadInflight(sess.ID); err != nil {
		t.Fatalf("read inflight: %v", err)
	} else if ok {
		t.Fatal("reclaimed turn left an inflight sidecar behind; it would be re-dispatched at boot")
	}
}

// TestChatTurnIdleFallsBackToTunable pins that the sub-minute override is a
// test-only escape hatch: with no override set the configured tunable is what
// bounds a turn, so nothing about production timing changed.
func TestChatTurnIdleFallsBackToTunable(t *testing.T) {
	tun := agent.NewTunables()
	tun.SetChatTurnIdleTimeoutMinutes(7)
	s := &Server{tun: tun}
	if got, want := s.chatTurnIdle(), 7*time.Minute; got != want {
		t.Fatalf("chatTurnIdle() = %v, want %v", got, want)
	}
	s.chatTurnIdleOverride = 250 * time.Millisecond
	if got, want := s.chatTurnIdle(), 250*time.Millisecond; got != want {
		t.Fatalf("overridden chatTurnIdle() = %v, want %v", got, want)
	}
}

// unmarshalStepsForTest decodes a message's persisted step blob, failing the test
// on a malformed payload rather than silently reporting zero steps (which would
// make an assertion about persisted error cards pass for the wrong reason).
func unmarshalStepsForTest(t *testing.T, raw string) []agent.TurnStep {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var steps []agent.TurnStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		t.Fatalf("decode steps %q: %v", raw, err)
	}
	return steps
}
