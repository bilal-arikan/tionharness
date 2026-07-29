package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TestOptimizerLog_RoundTrip covers the claude-cli seam: the shell runner records
// under the output it returned, and the trace conversion looks the SAME text up
// later. Lookup must be non-destructive, because both the live OnEvent path and
// the final batch conversion resolve the same step.
func TestOptimizerLog_RoundTrip(t *testing.T) {
	var l optimizerLog
	opt := tools.ShellOptimization{Kind: "sqz", InTokens: 841, OutTokens: 57}
	l.record("compressed body", opt)

	for i := 0; i < 2; i++ {
		got := l.lookup("compressed body")
		if got == nil {
			t.Fatalf("lookup %d: recorded optimization not found — the chip would silently vanish", i)
		}
		if got.Kind != "sqz" || got.InTokens != 841 || got.OutTokens != 57 {
			t.Fatalf("lookup %d returned %+v, want %+v", i, *got, opt)
		}
	}
	if l.lookup("something else entirely") != nil {
		t.Fatal("unrecorded output must not resolve to an optimization")
	}
}

// TestOptimizerLog_TrimsWhitespace locks the transport tolerance: the MCP result
// → CLI tool_result hop may add or drop a trailing newline, and a chip that
// disappeared over one "\n" would be a maddening bug to chase.
func TestOptimizerLog_TrimsWhitespace(t *testing.T) {
	var l optimizerLog
	l.record("body\n", tools.ShellOptimization{Kind: "rtk"})
	if l.lookup("body") == nil {
		t.Fatal("lookup must tolerate a trailing newline added/dropped in transit")
	}
	if l.lookup("  body  \n\n") == nil {
		t.Fatal("lookup must tolerate surrounding whitespace")
	}
}

// TestOptimizerLog_Bounded proves the ring evicts: the log lives for the whole
// runtime, so an unbounded map would grow with every shell call of every session.
func TestOptimizerLog_Bounded(t *testing.T) {
	var l optimizerLog
	for i := 0; i < optimizerLogSize*2; i++ {
		l.record(strings.Repeat("x", i+1), tools.ShellOptimization{Kind: "sqz", InTokens: 10, OutTokens: 1})
	}
	if len(l.index) > optimizerLogSize {
		t.Fatalf("index grew to %d entries, ring size is %d — the log is unbounded", len(l.index), optimizerLogSize)
	}
	// The most recent entry survives; the very first is long gone.
	if l.lookup(strings.Repeat("x", optimizerLogSize*2)) == nil {
		t.Fatal("newest entry must still resolve")
	}
	if l.lookup("x") != nil {
		t.Fatal("oldest entry should have been evicted")
	}
}

// TestOptimizerLog_IgnoresEmpty: a blank output carries no identity, so recording
// it would make every other blank result falsely claim an optimization.
func TestOptimizerLog_IgnoresEmpty(t *testing.T) {
	var l optimizerLog
	l.record("   \n", tools.ShellOptimization{Kind: "sqz"})
	if l.lookup("") != nil || l.lookup("  ") != nil {
		t.Fatal("empty output must never resolve to an optimization")
	}
}
