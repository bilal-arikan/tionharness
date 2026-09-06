package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestToolCallCountFromSteps verifies AddMessage sums an assistant message's tool
// steps into Session.ToolCallCount, that user/system messages contribute nothing,
// and that the count is recomputed from the transcript on reload (the header's
// on-disk value is stale by design).
func TestToolCallCountFromSteps(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	// A user turn moves no tool counter.
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "do it"}); err != nil {
		t.Fatalf("add user: %v", err)
	}
	// An assistant turn with two tool steps (+ a text step) adds 2.
	twoTools := `[{"kind":"text"},{"kind":"tool","tool":"Read"},{"kind":"tool","tool":"Bash"}]`
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "ok", Steps: twoTools}); err != nil {
		t.Fatalf("add assistant: %v", err)
	}
	// A second assistant turn with one tool step adds 1 → total 3.
	oneTool := `[{"kind":"tool","tool":"Grep"}]`
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "more", Steps: oneTool}); err != nil {
		t.Fatalf("add assistant 2: %v", err)
	}

	got, _ := d.GetSession(ctx, sess.ID)
	if got.ToolCallCount != 3 {
		t.Fatalf("ToolCallCount = %d, want 3", got.ToolCallCount)
	}
	_ = d.Close()

	// Reload: reconcileHeader must recompute the same total from the message lines.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	reloaded, _ := d2.GetSession(ctx, sess.ID)
	if reloaded.ToolCallCount != 3 {
		t.Fatalf("reloaded ToolCallCount = %d, want 3 (recomputed from lines)", reloaded.ToolCallCount)
	}
}

// TestActivityHookCarriesDeltas verifies the activity hook fires per append with
// the session's new totals and this append's deltas (prev = total - delta).
func TestActivityHookCarriesDeltas(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID})

	var got []ActivitySignal
	if err := d.SetActivityHook(func(sig ActivitySignal) error {
		got = append(got, sig)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "hi"}); err != nil {
		t.Fatalf("add user: %v", err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "ok",
		Steps: `[{"kind":"tool"},{"kind":"tool"}]`}); err != nil {
		t.Fatalf("add assistant: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("hook fired %d times, want 2", len(got))
	}
	// User append: message 1 (+1), tool total 0 (+0).
	if got[0].MessageTotal != 1 || got[0].MessageDelta != 1 || got[0].ToolDelta != 0 {
		t.Errorf("user signal = %+v", got[0])
	}
	// Assistant append: message 2 (+1), tool total 2 (+2).
	if got[1].MessageTotal != 2 || got[1].MessageDelta != 1 || got[1].ToolTotal != 2 || got[1].ToolDelta != 2 {
		t.Errorf("assistant signal = %+v", got[1])
	}
}

// TestCountToolSteps pins the tolerant scan: empty/"[]"/malformed inputs count as
// zero rather than erroring (a bad transcript line must not break persistence).
func TestCountToolSteps(t *testing.T) {
	cases := map[string]int{
		"":                  0,
		"[]":                0,
		`[{"kind":"text"}]`: 0,
		`[{"kind":"tool"}]`: 1,
		`[{"kind":"tool"},{"kind":"tool"},{"kind":"todo"}]`: 2,
		`not json`: 0,
	}
	for in, want := range cases {
		if got := countToolSteps(in); got != want {
			t.Errorf("countToolSteps(%q) = %d, want %d", in, got, want)
		}
	}
}
