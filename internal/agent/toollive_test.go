package agent

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func TestParallelBatchEmitterIsSerialized(t *testing.T) {
	var active atomic.Int32
	var overlaps atomic.Int32
	var emitted []TurnStep
	safeEmit := serializeStepEmitter(func(st TurnStep) {
		if active.Add(1) != 1 {
			overlaps.Add(1)
		}
		time.Sleep(time.Millisecond)
		emitted = append(emitted, st)
		active.Add(-1)
	})

	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			safeEmit(TurnStep{Batch: id})
		}(i)
	}
	wg.Wait()

	if got := overlaps.Load(); got != 0 {
		t.Fatalf("emitter overlapped %d times", got)
	}
	if len(emitted) != count {
		t.Fatalf("got %d emitted steps, want %d", len(emitted), count)
	}
}

func TestParallelBatchCancellationTombstonesEveryOpenCard(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	provider := &toolFakeProvider{script: []scriptedToolResp{{
		stop: providers.StopToolUse,
		toolCalls: []providers.ToolCall{
			{ID: "call-1", Name: "run_subagent", Input: json.RawMessage(`{"target":"missing","task":"one"}`)},
			{ID: "call-2", Name: "run_subagent", Input: json.RawMessage(`{"target":"missing","task":"two"}`)},
		},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var mu sync.Mutex
	var emitted []TurnStep
	_, _, err := rt.CompleteWithToolsStream(ctx, agent, provider, providers.Request{
		Messages: []providers.Message{{Role: providers.RoleUser, Text: "delegate"}},
	}, false, func(st TurnStep) {
		mu.Lock()
		emitted = append(emitted, st)
		mu.Unlock()
	})
	if err == nil {
		t.Fatal("cancelled batch returned nil error")
	}

	mu.Lock()
	defer mu.Unlock()
	tombstones := map[string]int{}
	for _, st := range emitted {
		if st.Kind == StepTombstone {
			tombstones[st.Ref]++
		}
	}
	for _, id := range []string{"call-1", "call-2"} {
		if tombstones[id] != 1 {
			t.Fatalf("card %q got %d tombstones, want 1; steps=%+v", id, tombstones[id], emitted)
		}
	}
}

func TestPanicTombstonesEveryOpenCardAndRepanics(t *testing.T) {
	var emitted []TurnStep
	cards := map[string]*liveCard{
		"call-1": openLive(func(st TurnStep) { emitted = append(emitted, st) }, "call-1", TurnStep{Kind: StepTool}),
		"call-2": openLive(func(st TurnStep) { emitted = append(emitted, st) }, "call-2", TurnStep{Kind: StepTool}),
	}

	func() {
		defer func() {
			if recovered := recover(); recovered != "tool panic" {
				t.Fatalf("recovered %v, want tool panic", recovered)
			}
		}()
		defer cancelLiveCardsOnPanic(&cards)
		panic("tool panic")
	}()

	tombstones := map[string]int{}
	for _, st := range emitted {
		if st.Kind == StepTombstone {
			tombstones[st.Ref]++
		}
	}
	for _, id := range []string{"call-1", "call-2"} {
		if tombstones[id] != 1 {
			t.Fatalf("card %q got %d tombstones, want 1; steps=%+v", id, tombstones[id], emitted)
		}
	}
}

func TestNonStreamingToolPublishesOpeningCard(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	provider := &toolFakeProvider{script: []scriptedToolResp{
		{
			stop: providers.StopToolUse,
			toolCalls: []providers.ToolCall{{
				ID:    "read-1",
				Name:  "Read",
				Input: json.RawMessage(`{"path":"missing.txt"}`),
			}},
		},
		{stop: providers.StopEndTurn, text: "done"},
	}}

	var emitted []TurnStep
	_, _, err := rt.CompleteWithToolsStream(
		context.Background(),
		agent,
		provider,
		providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "read"}}},
		false,
		func(st TurnStep) { emitted = append(emitted, st) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(emitted) < 2 {
		t.Fatalf("got %d emitted steps, want opening and final cards", len(emitted))
	}
	if first := emitted[0]; first.ID != "read-1" || first.Kind != StepTool || !first.Running || first.Append {
		t.Fatalf("first emitted step is not the opening card: %+v", first)
	}
	var final *TurnStep
	for i := range emitted {
		if emitted[i].ID == "read-1" && !emitted[i].Running {
			final = &emitted[i]
		}
	}
	if final == nil {
		t.Fatal("final replacement card was not emitted")
	}
	if final.Append {
		t.Fatalf("final card must not carry append: %+v", *final)
	}
}
