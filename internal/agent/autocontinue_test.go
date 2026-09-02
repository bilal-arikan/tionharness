package agent

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func todoStep(statuses ...string) TurnStep {
	items := make([]TodoItem, len(statuses))
	for i, s := range statuses {
		items[i] = TodoItem{Content: "t", Status: s}
	}
	return TurnStep{Kind: StepTodo, Todos: items}
}

func TestNeedsAutoContinue(t *testing.T) {
	cases := []struct {
		name  string
		steps []TurnStep
		want  bool
	}{
		{"empty", nil, false},
		{"plain text answer", []TurnStep{{Kind: StepText, Text: "done"}}, false},
		{
			"open todo (in_progress)",
			[]TurnStep{{Kind: StepText}, todoStep("completed", "in_progress", "pending")},
			true,
		},
		{
			"all todos completed",
			[]TurnStep{{Kind: StepText}, todoStep("completed", "completed")},
			false,
		},
		{
			"ends on ToolSearch activation",
			[]TurnStep{{Kind: StepText, Text: "let me load tools"}, {Kind: StepTool, Tool: "ToolSearch"}},
			true,
		},
		{
			"ends on activate_tools then narration",
			[]TurnStep{{Kind: StepTool, Tool: "activate_tools"}, {Kind: StepText, Text: "loaded"}},
			true, // trailing narration is skipped; the last real action is the activation
		},
		{
			"ends on a normal tool call",
			[]TurnStep{{Kind: StepTool, Tool: "WebSearch"}, {Kind: StepText, Text: "here are results"}},
			false,
		},
		{
			"latest todo all done overrides an earlier open one",
			[]TurnStep{todoStep("pending"), {Kind: StepTool, Tool: "WebSearch"}, todoStep("completed")},
			false,
		},
	}
	for _, c := range cases {
		if got := needsAutoContinue(c.steps); got != c.want {
			t.Errorf("%s: needsAutoContinue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldAutoContinueSuppressedWhileWorkerRunning(t *testing.T) {
	rt := testRuntime(t)
	sessionID := "SES-coordinator"
	steps := []TurnStep{todoStep("in_progress")}

	rt.coordSlotFor(sessionID).workers.Add(1)
	if rt.shouldAutoContinue(sessionID, steps) {
		t.Fatal("auto-continue must be suppressed while a worker is running")
	}

	rt.coordSlotFor(sessionID).workers.Add(-1)
	if !rt.shouldAutoContinue(sessionID, steps) {
		t.Fatal("open work must trigger auto-continue after workers finish")
	}
}

// TestAutoContinueSkipsStoppedAndCutShortTurns pins the two suppressions the nudge
// must honour: a turn that was CUT SHORT (watchdog cut, iteration/guardrail ceiling)
// and a run the human STOPPED get no "you stopped without finishing" nudge. Both must
// return before ANY persistence — the runtime here carries no store, so a continuation
// that got as far as recording the nudge would panic.
func TestAutoContinueSkipsStoppedAndCutShortTurns(t *testing.T) {
	steps := []TurnStep{todoStep("in_progress")} // unfinished work: the nudge would otherwise fire

	cases := []struct {
		name      string
		truncated bool
		stopped   bool
	}{
		{name: "previous turn was cut short", truncated: true},
		{name: "run was stopped by hand", stopped: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := testRuntime(t)
			rt.tun = NewTunables()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if c.stopped {
				cancel() // the "Durdur" button
			}
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("auto-continue issued a continuation instead of standing down: %v", rec)
				}
			}()
			rt.maybeAutoContinue(ctx, db.Agent{ID: "AG1"}, "SES1", KindSpawn, steps, c.truncated)
		})
	}
}

// TestDeadlineExpired separates the two ways a parent context ends: an exhausted
// budget (which still earns the explanatory note) from a human stop (which stays
// silent).
func TestDeadlineExpired(t *testing.T) {
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if deadlineExpired(stopped) {
		t.Error("a hand-cancelled context is a stop, not an expired budget")
	}

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if !deadlineExpired(expired) {
		t.Error("an exhausted deadline must read as an expired budget")
	}

	cut, cancelCut := context.WithCancelCause(context.Background())
	cancelCut(ErrTurnIdleTimeout)
	if !deadlineExpired(cut) {
		t.Error("a watchdog cut must read as an expired budget")
	}
}

func TestHasToolStep(t *testing.T) {
	if hasToolStep([]TurnStep{{Kind: StepText}, {Kind: StepThinking}}) {
		t.Error("text/thinking only should have no tool progress")
	}
	if !hasToolStep([]TurnStep{{Kind: StepText}, {Kind: StepTool, Tool: "Bash"}}) {
		t.Error("a tool call is tool progress")
	}
	if !hasToolStep([]TurnStep{todoStep("pending")}) {
		t.Error("a todo write is tool progress")
	}
}
