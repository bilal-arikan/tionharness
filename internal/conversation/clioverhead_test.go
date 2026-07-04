package conversation

import "testing"

func TestPredictCLIOverhead(t *testing.T) {
	// Zero tools → just the fixed base floor (system prompt + built-in tools).
	if got := PredictCLIOverhead(0); got != CLIBaseTokens {
		t.Fatalf("PredictCLIOverhead(0) = %d, want %d", got, CLIBaseTokens)
	}
	// Negative tool counts are clamped to zero, not subtracted.
	if got := PredictCLIOverhead(-5); got != CLIBaseTokens {
		t.Fatalf("PredictCLIOverhead(-5) = %d, want %d (clamped)", got, CLIBaseTokens)
	}
	// Each bridged tool adds a linear per-tool schema cost on top of the base.
	if got, want := PredictCLIOverhead(35), CLIBaseTokens+35*CLIAvgBridgedToolTokens; got != want {
		t.Fatalf("PredictCLIOverhead(35) = %d, want %d", got, want)
	}
	// Sanity: the base floor must equal its two measured components.
	if CLIBaseTokens != CLIBaseSystemTokens+CLIBuiltinToolsTokens {
		t.Fatalf("CLIBaseTokens %d != system %d + builtins %d", CLIBaseTokens, CLIBaseSystemTokens, CLIBuiltinToolsTokens)
	}
}
