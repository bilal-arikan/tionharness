package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// hasPruneStep reports whether the trace carries the phase-1 prune card.
func hasPruneStep(steps []TurnStep) bool {
	for _, s := range steps {
		if s.Kind == StepCompaction && s.Trigger == conversation.TriggerPrune {
			return true
		}
	}
	return false
}

// TestLoop_PruneBeforeFold: an overflow whose history holds a big old tool
// result is recovered by dropping that body — no summarizer call at all. The
// saving is what the fold would have paid an LLM for, so taking it first is the
// whole point of the phase.
func TestLoop_PruneBeforeFold(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}

	big := strings.Repeat("log line padding\n", 2000) // ~34 KB, far over the prune threshold
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "run the tests"},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{{CallID: "c1", Content: big}}},
	}
	// Pad past the kept tail so the big result is old enough to prune.
	for i := 0; i < DefaultReactiveKeepRecent; i++ {
		msgs = append(msgs,
			providers.Message{Role: providers.RoleAssistant, Text: "a"},
			providers.Message{Role: providers.RoleUser, Text: "u"},
		)
	}

	fp := &fakeProvider{script: []scriptedResp{
		{err: errors.New("prompt is too long: 250000 tokens > 200000 maximum")}, // initial overflow
		{stop: providers.StopEndTurn, text: "recovered answer"},                 // retry on the pruned history
	}}

	resp, steps, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, providers.Request{Messages: msgs}, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Text != "recovered answer" {
		t.Errorf("final text = %q, want %q", resp.Text, "recovered answer")
	}
	if fp.calls != 2 {
		t.Errorf("provider calls = %d, want 2 (overflow + retry); a summarizer call means the free pass was skipped", fp.calls)
	}
	if !hasPruneStep(steps) {
		t.Errorf("no prune compaction step in trace: %+v", steps)
	}
	last := fp.requests[len(fp.requests)-1]
	if got := last.Messages[2].ToolResults[0].Content; !strings.HasPrefix(got, "[tool result pruned") {
		t.Errorf("retry still carried the full tool body (%d bytes)", len(got))
	}
	if last.Messages[2].ToolResults[0].CallID != "c1" {
		t.Errorf("prune broke tool_use/tool_result pairing")
	}
}

// TestLoop_PruneSkippedWithoutOversizedResults: a history with nothing worth
// pruning must fall straight through to the summarizer fold, exactly as before
// this phase existed.
func TestLoop_PruneSkippedWithoutOversizedResults(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}

	var msgs []providers.Message
	for i := 0; i < 6; i++ {
		msgs = append(msgs,
			providers.Message{Role: providers.RoleUser, Text: "u"},
			providers.Message{Role: providers.RoleAssistant, Text: "a"},
		)
	}
	fp := &fakeProvider{script: []scriptedResp{
		{err: errors.New("prompt is too long")},
		{stop: providers.StopEndTurn, text: "SUMMARY"},
		{stop: providers.StopEndTurn, text: "recovered answer"},
	}}

	_, steps, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, providers.Request{Messages: msgs}, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if fp.calls != 3 {
		t.Errorf("provider calls = %d, want 3 (overflow + summarize + retry)", fp.calls)
	}
	if hasPruneStep(steps) {
		t.Errorf("prune card emitted with nothing to prune: %+v", steps)
	}
}
