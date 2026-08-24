package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// backstopEngine builds an AutomationEngine over a temp store. The runtime is real
// so guardsPass can publish its event without a nil dereference.
func backstopEngine(t *testing.T) *AutomationEngine {
	t.Helper()
	r := lifecycleRuntime(t)
	return NewAutomationEngine(r.db, r, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// seedAutomation stores an automation and returns it with its assigned id.
func seedAutomation(t *testing.T, e *AutomationEngine, a db.Automation) db.Automation {
	t.Helper()
	if a.PromptTemplate == "" {
		a.PromptTemplate = "go"
	}
	if a.TriggerTag == "" {
		a.TriggerTag = "loop"
	}
	a.Enabled = true
	created, err := e.db.CreateAutomation(context.Background(), a)
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}
	return created
}

// enabled reports the automation's stored enabled flag.
func enabled(t *testing.T, e *AutomationEngine, id string) bool {
	t.Helper()
	got, err := e.db.GetAutomation(context.Background(), id)
	if err != nil {
		t.Fatalf("get automation: %v", err)
	}
	return got.Enabled
}

// TestGuardsPass_AbsoluteBackstop covers the third defence layer: automations
// ALREADY on disk with MaxIterations <= 0. They never pass through
// db.ValidateMaxIterations — they predate the rule, arrived in a market package,
// or were hand-edited — so without this brake they loop with no lifetime bound at
// all. Four such rows existed when this landed, three of them enabled.
func TestGuardsPass_AbsoluteBackstop(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()

	// At the backstop: stopped and auto-disabled.
	at := seedAutomation(t, e, db.Automation{MaxIterations: 0, IterationCount: db.AbsoluteIterationBackstop})
	if e.guardsPass(ctx, at) {
		t.Error("an unbounded automation at the backstop must not fire")
	}
	if enabled(t, e, at.ID) {
		t.Error("reaching the backstop must auto-disable the automation, not merely skip one fire")
	}

	// Below it: still runs. The brake must not punish legacy data earlier than an
	// explicit maximum would — a board automation firing on card moves is
	// legitimately open-ended until the backstop.
	below := seedAutomation(t, e, db.Automation{MaxIterations: 0, IterationCount: db.AbsoluteIterationBackstop - 1})
	if !e.guardsPass(ctx, below) {
		t.Error("an unbounded automation below the backstop must still fire")
	}
	if !enabled(t, e, below.ID) {
		t.Error("an automation below the backstop must stay enabled")
	}

	// A negative count is the same "unlimited" shape and must be treated alike.
	neg := seedAutomation(t, e, db.Automation{MaxIterations: -5, IterationCount: db.AbsoluteIterationBackstop})
	if e.guardsPass(ctx, neg) {
		t.Error("a negative maxIterations is the unlimited shape too; the backstop must apply")
	}
}

// TestGuardsPass_ExplicitLimitUnchanged: adding the backstop must not disturb the
// ordinary bounded path, which is what nearly every automation uses.
func TestGuardsPass_ExplicitLimitUnchanged(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()

	spent := seedAutomation(t, e, db.Automation{MaxIterations: 3, IterationCount: 3})
	if e.guardsPass(ctx, spent) {
		t.Error("an automation at its explicit limit must not fire")
	}
	if enabled(t, e, spent.ID) {
		t.Error("hitting the explicit limit must auto-disable")
	}

	left := seedAutomation(t, e, db.Automation{MaxIterations: 3, IterationCount: 1})
	if !e.guardsPass(ctx, left) {
		t.Error("an automation with budget left must fire")
	}

	// A bounded automation well past the BACKSTOP number is still governed by its
	// own limit — the backstop is only for the unbounded case, so a hypothetical
	// row with a huge count must not be re-judged by it.
	big := seedAutomation(t, e, db.Automation{MaxIterations: db.AbsoluteIterationBackstop + 50, IterationCount: db.AbsoluteIterationBackstop + 10})
	if !e.guardsPass(ctx, big) {
		t.Error("an explicitly-bounded automation must be judged by its own limit, not the backstop")
	}
}
