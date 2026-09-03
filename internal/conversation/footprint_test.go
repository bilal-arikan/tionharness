package conversation

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestEstimateToolDefTokensCountsSchema(t *testing.T) {
	defs := []providers.ToolDef{{
		Name:        "read_file",
		Description: "Read a file from disk and return its contents.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}}
	got := EstimateToolDefTokens(defs)
	if got <= msgOverhead {
		t.Fatalf("EstimateToolDefTokens = %d, want the schema to contribute beyond overhead", got)
	}
	if EstimateToolDefTokens(nil) != 0 {
		t.Errorf("no defs must cost nothing")
	}
}

// The whole point of the in-flight figure is that it sees the tools block the
// message-only estimate is blind to.
func TestEstimateInFlightTokensExceedsMessagesAlone(t *testing.T) {
	msgs := []providers.Message{{Role: providers.RoleUser, Text: "hello"}}
	defs := []providers.ToolDef{{
		Name:        "grep",
		Description: "Search file contents with a regular expression.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}}}`),
	}}
	msgsOnly := EstimateProviderTokens(msgs)
	inFlight := EstimateInFlightTokens(msgs, defs)
	if inFlight <= msgsOnly {
		t.Fatalf("in-flight %d must exceed messages-only %d once tools are shipped", inFlight, msgsOnly)
	}
	if want := msgsOnly + EstimateToolDefTokens(defs); inFlight != want {
		t.Errorf("in-flight = %d, want %d (messages + tools)", inFlight, want)
	}
}

func TestPruneMinBytesForScalesWithWindow(t *testing.T) {
	if got := PruneMinBytesFor(0); got != pruneToolResultMinBytes {
		t.Errorf("unknown window = %d, want the default %d", got, pruneToolResultMinBytes)
	}
	if got := PruneMinBytesFor(200_000); got != pruneToolResultMinBytes {
		t.Errorf("reference window = %d, want the default %d", got, pruneToolResultMinBytes)
	}
	small, large := PruneMinBytesFor(32_000), PruneMinBytesFor(1_000_000)
	if small >= pruneToolResultMinBytes {
		t.Errorf("32K window = %d, want below the 200K default", small)
	}
	if large <= pruneToolResultMinBytes {
		t.Errorf("1M window = %d, want above the 200K default", large)
	}
	if small < pruneMinBytesFloor || large > pruneMinBytesCeil {
		t.Errorf("bounds violated: small=%d floor=%d large=%d ceil=%d",
			small, pruneMinBytesFloor, large, pruneMinBytesCeil)
	}
}

// A small window must actually prune bodies the flat 4 KB threshold would keep —
// the concrete gap the scaling exists to close.
func TestPruneInFlightToolResultsMinHonoursSmallWindow(t *testing.T) {
	body := make([]byte, 3000)
	for i := range body {
		body[i] = 'x'
	}
	msgs := []providers.Message{
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{{CallID: "a", Content: string(body)}}},
		{Role: providers.RoleAssistant, Text: "ok"},
		{Role: providers.RoleUser, Text: "next"},
	}
	if _, _, ok := PruneInFlightToolResults(msgs, 1); ok {
		t.Fatalf("3 KB body must NOT qualify under the flat 4 KB default")
	}
	out, stat, ok := PruneInFlightToolResultsMin(msgs, 1, PruneMinBytesFor(32_000))
	if !ok || stat.Pruned != 1 {
		t.Fatalf("scaled threshold must prune the 3 KB body: ok=%v pruned=%d", ok, stat.Pruned)
	}
	if msgs[0].ToolResults[0].Content != string(body) {
		t.Errorf("input slice must not be mutated")
	}
	if out[0].ToolResults[0].CallID != "a" {
		t.Errorf("CallID must survive the prune so tool_use pairing holds")
	}
}
