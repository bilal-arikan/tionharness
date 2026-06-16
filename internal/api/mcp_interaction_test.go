package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal/swarmgo/internal/tools"
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
	run := runs.register("r1", func() {})
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

func TestInteractionBackend_AskTurnEnded(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r2", func() {})
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
	run := runs.register("r3", func() {})
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
	run := runs.register("rc", func() {})
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

func (f *fakeSink) CreateArtifact(_ context.Context, title, kind, language, content string) (tools.ArtifactRef, error) {
	f.created++
	return tools.ArtifactRef{ID: "art-1", Title: title, Kind: kind, Version: 1}, nil
}
func (f *fakeSink) UpdateArtifact(_ context.Context, id, content, note string) (tools.ArtifactRef, error) {
	f.updated++
	return tools.ArtifactRef{ID: id, Title: "t", Kind: "markdown", Version: 2}, nil
}

func TestInteractionBackend_Artifact(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("ra", func() {})
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

func TestInteractionBackend_UnknownToken(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs}
	if _, err := b.Call(context.Background(), "ghost", "ask_user", json.RawMessage(`{"question":"q"}`)); err == nil {
		t.Fatal("want error for unknown token")
	}
}
