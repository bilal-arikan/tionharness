package agent

import (
	"testing"

	"github.com/bilal/swarmgo/internal/providers"
)

// TestFillCancelledResults_BalancesEveryToolUse verifies A3: after a mid-batch
// cancellation, every tool_use call gets exactly one tool_result — the ones that
// ran keep theirs, the rest get a synthetic 'cancelled' result — so the
// in-flight history carries no dangling tool_use.
func TestFillCancelledResults_BalancesEveryToolUse(t *testing.T) {
	calls := []providers.ToolCall{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	// Only the first call completed before cancellation.
	results := []providers.ToolResult{{CallID: "a", Content: "ok"}}

	out := fillCancelledResults(results, calls)

	if len(out) != len(calls) {
		t.Fatalf("expected one result per call (%d), got %d", len(calls), len(out))
	}
	byID := map[string]providers.ToolResult{}
	for _, r := range out {
		byID[r.CallID] = r
	}
	for _, c := range calls {
		r, ok := byID[c.ID]
		if !ok {
			t.Fatalf("call %q has no tool_result (dangling tool_use)", c.ID)
		}
		if c.ID == "a" {
			if r.IsError || r.Content != "ok" {
				t.Errorf("completed call %q should keep its real result, got %+v", c.ID, r)
			}
			continue
		}
		if !r.IsError || r.Content != cancelledToolMsg {
			t.Errorf("abandoned call %q should carry the synthetic cancelled result, got %+v", c.ID, r)
		}
	}
}

// TestFillCancelledResults_NoDuplicates ensures a call that already has a result
// is not given a second (synthetic) one.
func TestFillCancelledResults_NoDuplicates(t *testing.T) {
	calls := []providers.ToolCall{{ID: "x"}}
	results := []providers.ToolResult{{CallID: "x", Content: "done"}}
	out := fillCancelledResults(results, calls)
	if len(out) != 1 {
		t.Fatalf("expected no duplicate result, got %d", len(out))
	}
}
