package conversation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// bigOutput builds a tool result body of n bytes with realistic line breaks, so
// the marker's line count is exercised alongside its size.
func bigOutput(n int) string {
	line := strings.Repeat("x", 63) + "\n"
	return strings.Repeat(line, n/len(line)+1)
}

// toolResultMsg is the shape the tool loop appends after a batch: a user turn
// carrying nothing but results.
func toolResultMsg(results ...providers.ToolResult) providers.Message {
	return providers.Message{Role: providers.RoleUser, ToolResults: results}
}

func TestPruneInFlightToolResultsReplacesOnlyOversizedOldBodies(t *testing.T) {
	small := "exit 0"
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "run the tests"},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c1", Content: big}),
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c2", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c2", Content: small}),
		{Role: providers.RoleAssistant, Text: "done"},
	}
	out, stat, ok := PruneInFlightToolResults(msgs, 1)
	if !ok {
		t.Fatalf("expected a prune, got ok=false")
	}
	if stat.Pruned != 1 {
		t.Fatalf("pruned = %d, want 1", stat.Pruned)
	}
	if got := out[2].ToolResults[0].Content; !strings.HasPrefix(got, prunedToolResultPrefix) {
		t.Fatalf("oversized body not replaced: %q", got)
	}
	if got := out[4].ToolResults[0].Content; got != small {
		t.Fatalf("small body was rewritten: %q", got)
	}
	// Identity must survive: the pairing repair keys off CallID.
	if out[2].ToolResults[0].CallID != "c1" {
		t.Fatalf("CallID lost: %q", out[2].ToolResults[0].CallID)
	}
	if stat.AfterTokens >= stat.BeforeTokens {
		t.Fatalf("no saving: %d -> %d", stat.BeforeTokens, stat.AfterTokens)
	}
	if stat.SavedBytes <= 0 {
		t.Fatalf("SavedBytes = %d, want > 0", stat.SavedBytes)
	}
}

func TestPruneInFlightToolResultsKeepsRecentTail(t *testing.T) {
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c1", Content: big}),
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c2", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c2", Content: big}),
	}
	out, stat, ok := PruneInFlightToolResults(msgs, 2)
	if !ok || stat.Pruned != 1 {
		t.Fatalf("ok=%v pruned=%d, want a single prune outside the kept tail", ok, stat.Pruned)
	}
	if out[3].ToolResults[0].Content != big {
		t.Fatalf("protected tail was pruned")
	}
}

func TestPruneInFlightToolResultsNoopWhenNothingQualifies(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c1", Content: "exit 0"}),
		{Role: providers.RoleAssistant, Text: "done"},
	}
	out, stat, ok := PruneInFlightToolResults(msgs, 1)
	if ok {
		t.Fatalf("expected ok=false when every body is under the threshold")
	}
	if stat != (PruneStat{}) {
		t.Fatalf("stat should be zero on a no-op: %+v", stat)
	}
	if &out[0] != &msgs[0] {
		t.Fatalf("no-op should return the original slice")
	}
}

func TestPruneInFlightToolResultsDoesNotMutateInput(t *testing.T) {
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c1", Content: big}),
		{Role: providers.RoleAssistant, Text: "done"},
	}
	if _, _, ok := PruneInFlightToolResults(msgs, 1); !ok {
		t.Fatalf("expected a prune")
	}
	if msgs[1].ToolResults[0].Content != big {
		t.Fatalf("input slice was mutated in place")
	}
}

func TestPruneInFlightToolResultsIsIdempotent(t *testing.T) {
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		toolResultMsg(providers.ToolResult{CallID: "c1", Content: big}),
		{Role: providers.RoleAssistant, Text: "done"},
	}
	once, _, ok := PruneInFlightToolResults(msgs, 1)
	if !ok {
		t.Fatalf("expected a first prune")
	}
	if _, _, ok := PruneInFlightToolResults(once, 1); ok {
		t.Fatalf("second pass pruned again; the marker is not recognised")
	}
}

func TestPruneInFlightToolResultsSkipsVerbatimRawContent(t *testing.T) {
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{ID: "c1", Name: "Bash"}}},
		{
			Role:        providers.RoleUser,
			ToolResults: []providers.ToolResult{{CallID: "c1", Content: big}},
			RawContent:  json.RawMessage(`[{"type":"tool_result"}]`),
		},
		{Role: providers.RoleAssistant, Text: "done"},
	}
	if _, _, ok := PruneInFlightToolResults(msgs, 1); ok {
		t.Fatalf("a RawContent message reaches the provider verbatim; pruning it reports a phantom saving")
	}
}

func TestPruneInFlightToolResultsNoopWhenTailCoversEverything(t *testing.T) {
	big := bigOutput(pruneToolResultMinBytes * 2)
	msgs := []providers.Message{toolResultMsg(providers.ToolResult{CallID: "c1", Content: big})}
	if _, _, ok := PruneInFlightToolResults(msgs, 4); ok {
		t.Fatalf("expected ok=false when keepRecent covers the whole slice")
	}
}

func TestPruneSufficient(t *testing.T) {
	stat := PruneStat{AfterTokens: 1000}
	if !PruneSufficient(stat, 0) {
		t.Fatalf("unknown window should not force a summarizer call")
	}
	if !PruneSufficient(stat, 2000) {
		t.Fatalf("1000 tokens fits under 70%% of a 2000-token window")
	}
	if PruneSufficient(stat, 1200) {
		t.Fatalf("1000 tokens exceeds 70%% of a 1200-token window; the fold must still run")
	}
}
