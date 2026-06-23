package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/interaction"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// captureRun installs a write capture on a run and returns the collected events.
func captureRun(run *chatRun) (*[]capturedStep, *sync.Mutex) {
	var mu sync.Mutex
	var steps []capturedStep
	run.setWrite(func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		steps = append(steps, capturedStep{event: event, data: b})
	})
	return &steps, &mu
}

type capturedStep struct {
	event string
	data  json.RawMessage
}

func TestInteractionBackend_AskRoundTrip(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r1", "s-r1", func() {})
	defer runs.unregister("r1")
	steps, mu := captureRun(run)

	b := &interactionBackend{runs: runs}
	if !b.Valid(run.token) {
		t.Fatal("token should be valid")
	}
	if b.Valid("nope") {
		t.Fatal("unknown token should be invalid")
	}

	// Answer shortly after the call blocks.
	go func() {
		time.Sleep(20 * time.Millisecond)
		run.answer <- "BLUE"
	}()

	res, err := b.Call(context.Background(), run.token, "ask_user", json.RawMessage(`{"question":"color?","options":["RED","BLUE"]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "BLUE" {
		t.Fatalf("want answer BLUE, got %+v", res)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*steps) != 1 {
		t.Fatalf("want 1 emitted step, got %d", len(*steps))
	}
	var st struct {
		Kind    string   `json:"kind"`
		Text    string   `json:"text"`
		Options []string `json:"options"`
	}
	if err := json.Unmarshal((*steps)[0].data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Kind != "ask" || st.Text != "color?" || len(st.Options) != 2 {
		t.Fatalf("unexpected ask step: %+v", st)
	}
}

// TestInteractionAdvertisedNames locks the single-source invariant (CLI-2): the
// names handed to the CLI allowlist (InteractionEndpoint.ToolNames) are exactly
// the names the backend advertises via Tools(). Adding a tool to one therefore
// adds it to the other automatically. Also asserts the use_skill bridge is
// advertised and that self-manage-gated spawn_session is absent without a tun.
func TestInteractionAdvertisedNames(t *testing.T) {
	b := &interactionBackend{runs: newChatRuns()} // tun nil → self-manage off
	specs := b.Tools("") // no token → static set only (no per-run bridge)
	want := make(map[string]bool, len(specs))
	for _, s := range specs {
		want[s.Name] = true
	}
	got := interactionAdvertisedNames(nil)
	if len(got) != len(specs) {
		t.Fatalf("advertised names (%d) must match Tools() specs (%d)", len(got), len(specs))
	}
	for _, n := range got {
		if !want[n] {
			t.Fatalf("advertised name %q is not in Tools()", n)
		}
	}
	names := strings.Join(got, ",")
	if !contains(got, "use_skill") {
		t.Fatalf("use_skill must be advertised (CLI skill bridge); got %s", names)
	}
	if contains(got, "spawn_session") {
		t.Fatalf("spawn_session must NOT be advertised without self-manage; got %s", names)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestInteractionBridge covers CLI-3: a run with bridged self-management tools
// installed advertises them in Tools(token) and dispatches them via Call's
// default case through the run's bridge dispatcher.
func TestInteractionBridge(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rb", "s-rb", func() {})
	defer runs.unregister("rb")

	var gotName string
	var gotArgs string
	run.setBridge(
		[]providers.ToolDef{{Name: "create_agent", Description: "make an agent", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		func(_ context.Context, name string, args json.RawMessage) (string, error) {
			gotName, gotArgs = name, string(args)
			return "agent created", nil
		},
	)

	b := &interactionBackend{runs: runs}

	// Advertised for this token: the bridged tool appears alongside the static set.
	if !specHasTool(b.Tools(run.token), "create_agent") {
		t.Fatal("bridged tool create_agent must be advertised for the run token")
	}
	// Not advertised for an unknown token (no run → static set only).
	if specHasTool(b.Tools("other"), "create_agent") {
		t.Fatal("bridged tool must not leak to a different token")
	}

	// Dispatched through the bridge (namespaced name is stripped before dispatch).
	res, err := b.Call(context.Background(), run.token, "mcp__swarmgo_interaction__create_agent", json.RawMessage(`{"name":"x"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "agent created" {
		t.Fatalf("want bridged result, got %+v", res)
	}
	if gotName != "create_agent" || gotArgs != `{"name":"x"}` {
		t.Fatalf("bridge dispatch got name=%q args=%q", gotName, gotArgs)
	}
}

func specHasTool(specs []interaction.ToolSpec, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func TestInteractionBackend_AskTurnEnded(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r2", "s-r2", func() {})
	captureRun(run)
	b := &interactionBackend{runs: runs}

	// End the turn while the ask is blocked; the call must unblock with an error.
	go func() {
		time.Sleep(20 * time.Millisecond)
		runs.unregister("r2")
	}()
	res, err := b.Call(context.Background(), run.token, "ask_user", json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want error result after turn ended, got %+v", res)
	}
}

func TestInteractionBackend_Todo(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r3", "s-r3", func() {})
	defer runs.unregister("r3")
	steps, mu := captureRun(run)
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "todo_write",
		json.RawMessage(`{"todos":[{"content":"step one","status":"in_progress"},{"content":"step two","status":"pending"}]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "Checklist updated") {
		t.Fatalf("todo should succeed with confirmation text: %+v", res)
	}
	// No live emit: the CLI trace surfaces the checklist card.
	mu.Lock()
	defer mu.Unlock()
	if len(*steps) != 0 {
		t.Fatalf("todo should not emit a live step, got %d", len(*steps))
	}
}

func TestInteractionBackend_Confirm(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rc", "s-rc", func() {})
	defer runs.unregister("rc")
	captureRun(run)
	b := &interactionBackend{runs: runs}

	go func() {
		time.Sleep(20 * time.Millisecond)
		run.answer <- "Onayla"
	}()
	res, err := b.Call(context.Background(), run.token, "mcp__swarmgo_interaction__request_confirmation",
		json.RawMessage(`{"question":"Delete the file?"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Text != "confirmed" {
		t.Fatalf("want confirmed, got %+v", res)
	}
}

// fakeSink is a minimal ArtifactSink for the artifact dispatch test.
type fakeSink struct{ created, updated int }

func (f *fakeSink) CreateArtifact(_ context.Context, spec tools.CreateArtifactSpec) (tools.ArtifactRef, error) {
	f.created++
	return tools.ArtifactRef{ID: "art-1", Title: spec.Title, Kind: spec.Kind}, nil
}
func (f *fakeSink) UpdateArtifact(_ context.Context, id, content string) (tools.ArtifactRef, error) {
	f.updated++
	return tools.ArtifactRef{ID: id, Title: "t", Kind: "markdown"}, nil
}

func TestInteractionBackend_Artifact(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("ra", "s-ra", func() {})
	defer runs.unregister("ra")
	sink := &fakeSink{}
	run.setArtifacts(sink)
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "create_artifact",
		json.RawMessage(`{"title":"Doc","kind":"markdown","content":"# Hi"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || sink.created != 1 || !strings.Contains(res.Text, "art-1") {
		t.Fatalf("create_artifact failed: %+v created=%d", res, sink.created)
	}

	// No sink installed -> graceful error result.
	run.setArtifacts(nil)
	res2, _ := b.Call(context.Background(), run.token, "update_artifact",
		json.RawMessage(`{"id":"art-1","content":"x"}`))
	if !res2.IsError {
		t.Fatalf("want error when no sink, got %+v", res2)
	}
}

// TestInteractionBackend_AutonomousAskBails locks the headless-run guarantee: an
// autonomous run (scheduler/spawn) has no live client, so ask_user must
// return an error result at once instead of blocking until the 15-minute timeout.
func TestInteractionBackend_AutonomousAskBails(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rauto", "s-rauto", func() {})
	defer runs.unregister("rauto")
	run.autonomous = true
	b := &interactionBackend{runs: runs}

	res, err := b.Call(context.Background(), run.token, "ask_user", json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("autonomous ask_user must bail with an error result, got %+v", res)
	}
}

func TestInteractionBackend_UnknownToken(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs}
	if _, err := b.Call(context.Background(), "ghost", "ask_user", json.RawMessage(`{"question":"q"}`)); err == nil {
		t.Fatal("want error for unknown token")
	}
}
