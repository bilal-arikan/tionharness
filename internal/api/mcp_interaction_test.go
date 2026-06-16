package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
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
	if res.IsError {
		t.Fatalf("todo should succeed: %+v", res)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*steps) != 1 {
		t.Fatalf("want 1 emitted todo step, got %d", len(*steps))
	}
	var st struct {
		Kind  string `json:"kind"`
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	if err := json.Unmarshal((*steps)[0].data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Kind != "todo" || len(st.Todos) != 2 {
		t.Fatalf("unexpected todo step: %+v", st)
	}
}

func TestInteractionBackend_UnknownToken(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs}
	if _, err := b.Call(context.Background(), "ghost", "ask_user", json.RawMessage(`{"question":"q"}`)); err == nil {
		t.Fatal("want error for unknown token")
	}
}
