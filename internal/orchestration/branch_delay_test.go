package orchestration

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestBranchArmMatches_Modes covers the three documented branch match modes
// (contains/equals/regex) plus the empty-mode fall-back to contains. Only the
// regex mode was exercised before (via the GAN loop); equals/contains had no
// direct coverage. See _Docs/15-FLOW-CANVAS.md (Node.MatchMode).
func TestBranchArmMatches_Modes(t *testing.T) {
	cases := []struct {
		name, mode, pattern, value string
		want                       bool
	}{
		{"contains substring ci", "contains", "SHIP", "verdict: ship it", true},
		{"contains no match", "contains", "pivot", "verdict: ship", false},
		{"empty mode falls back to contains", "", "ship", "VERDICT: SHIP", true},
		{"equals trimmed ci match", "equals", "ship", "  SHIP \n", true},
		{"equals substring is not enough", "equals", "ship", "ship it", false},
		{"regex anchored match", "regex", `(?m)^VERDICT:\s*SHIP`, "VERDICT:  SHIP", true},
		{"regex no match", "regex", `^SHIP$`, "VERDICT: SHIP", false},
		{"invalid regex never matches", "regex", "(", "anything", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lower := strings.ToLower(c.value)
			trimmed := strings.ToLower(strings.TrimSpace(c.value))
			if got := branchArmMatches(c.mode, c.pattern, c.value, lower, trimmed); got != c.want {
				t.Errorf("branchArmMatches(%q,%q,%q) = %v, want %v", c.mode, c.pattern, c.value, got, c.want)
			}
		})
	}
}

// TestEvalBranch_DefaultAndNoMatch verifies the routing contract: the empty
// (default) arm wins only when no other arm matches regardless of its position,
// and a graph with no default and no match yields the "no match" sentinel.
func TestEvalBranch_DefaultAndNoMatch(t *testing.T) {
	// Default arm listed first, but a real match must still take priority.
	withDefault := Node{MatchMode: "contains", Branches: []Branch{
		{Contains: "", Next: "fallback"},
		{Contains: "ship", Next: "done"},
	}}
	if next, label := evalBranch(withDefault, "no verdict here"); next != "fallback" || label != "default" {
		t.Errorf("expected default arm, got next=%q label=%q", next, label)
	}
	if next, _ := evalBranch(withDefault, "please SHIP now"); next != "done" {
		t.Errorf("expected matching arm to win over default, got %q", next)
	}
	// No default and no match → empty next + "no match" label (run ends).
	noDefault := Node{MatchMode: "contains", Branches: []Branch{{Contains: "ship", Next: "done"}}}
	if next, label := evalBranch(noDefault, "pivot"); next != "" || label != "no match" {
		t.Errorf("expected no-match sentinel, got next=%q label=%q", next, label)
	}
}

// TestRunDelay_WaitsThenContinues verifies a delay node actually waits (bounded),
// records its "waited Nms" trace, and advances to Next. The delay node had no
// coverage at all. See _Docs/15-FLOW-CANVAS.md (delay node).
func TestRunDelay_WaitsThenContinues(t *testing.T) {
	g := Graph{
		Start: "d",
		Nodes: []Node{
			{ID: "d", Type: NodeDelay, DelayMs: 15, Next: "t", Title: "wait"},
			{ID: "t", Type: NodeTransform, Template: "go {{input}}", Next: ""},
		},
	}
	eng := NewEngine(okRunner{})
	start := time.Now()
	final, err := eng.Run(context.Background(), g, "ok", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Errorf("delay node returned too fast (%v) — did it actually wait?", elapsed)
	}
	if final.Current != "" {
		t.Errorf("expected finished run, got Current=%q", final.Current)
	}
	if final.Last != "go ok" {
		t.Errorf("expected the transform after the delay to run, got Last=%q", final.Last)
	}
	// The delay node must have recorded its wait in the trace.
	var found bool
	for _, e := range final.Trace {
		if e.NodeID == "d" && e.Output == "waited 15ms" {
			found = true
		}
	}
	if !found {
		t.Errorf("delay node trace entry missing, trace=%+v", final.Trace)
	}
}

// TestRunDelay_ContextCancel verifies a delay node honours context cancellation
// and surfaces it as a node error instead of blocking for the full duration.
func TestRunDelay_ContextCancel(t *testing.T) {
	g := Graph{Start: "d", Nodes: []Node{{ID: "d", Type: NodeDelay, DelayMs: 60000, Next: ""}}}
	eng := NewEngine(okRunner{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the run so the timer never elapses

	done := make(chan struct{})
	go func() {
		_, err := eng.Run(ctx, g, "in", NewState(g), nil)
		if err == nil || !strings.Contains(err.Error(), "delay") {
			t.Errorf("expected a delay error on cancelled context, got %v", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("delay node ignored context cancellation (blocked on the timer)")
	}
}

// TestSleepCtx_NonPositiveIsNoOp verifies the guard that a zero or negative
// delay returns immediately rather than arming a timer.
func TestSleepCtx_NonPositiveIsNoOp(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Errorf("zero delay should be a no-op, got %v", err)
	}
	if err := sleepCtx(context.Background(), -5); err != nil {
		t.Errorf("negative delay should be a no-op, got %v", err)
	}
}
