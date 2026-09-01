package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codexUsageHelperEnv names the fake-codex mode of the re-executed test binary.
// Anything credential-shaped would be stripped by the provider's hardened env,
// so the name is deliberately plain.
const codexUsageHelperEnv = "TIONHARNESS_FAKE_CODEX_MODE"

// TestCodexHelperFailsAfterMeasuredTurn impersonates a codex process that
// finished a measured turn and then reported a failure — the shape a late
// rejection or a cut stream has on the wire. It exits non-zero without going
// through the test framework so the parent sees a plain CLI crash.
func TestCodexHelperFailsAfterMeasuredTurn(t *testing.T) {
	if os.Getenv(codexUsageHelperEnv) != "fail-after-usage" {
		t.Skip("helper: only runs when re-executed as a fake codex binary")
	}
	fmt.Println(`{"type":"turn.started"}`)
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":5000,"cached_input_tokens":1200,` +
		`"cache_write_input_tokens":300,"output_tokens":440,"reasoning_output_tokens":180}}`)
	fmt.Println(`{"type":"error","message":"connection closed after the turn"}`)
	os.Exit(1)
}

// A codex turn that fails AFTER the provider measured it has really spent those
// tokens. The provider contract returns (nil, err), so unless the failure
// carries the usage the caller bills the turn as zero — which is exactly what
// the codex transport did: it never called WithUsage anywhere.
func TestCodexFailedTurnCarriesUsageOutOfTheProvider(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	t.Setenv(codexUsageHelperEnv, "fail-after-usage")

	c := &CodexCLI{binPath: self}
	args := []string{"-test.run=TestCodexHelperFailsAfterMeasuredTurn", "-test.v=false"}
	resp, _, err := c.runAttempt(context.Background(), args, "prompt", "gpt-test", Request{}, "")
	if err == nil {
		t.Fatalf("a reported stream error must fail the turn; resp=%+v", resp)
	}
	if resp != nil {
		t.Fatalf("a failed turn must not return a response: %+v", resp)
	}
	ue, ok := UsageFromError(err)
	if !ok {
		t.Fatalf("the codex failure dropped the turn's usage: %v", err)
	}
	// freshInput: the codex counters nest the cache subsets inside input_tokens.
	want := Usage{
		InputTokens:      5000 - 1200 - 300,
		OutputTokens:     440,
		CacheReadTokens:  1200,
		CacheWriteTokens: 300,
		ThinkingTokens:   180,
		// turn.completed reported reasoning_output_tokens before the failure, so
		// the 180 is measured, not derived. The flag has to survive onto the error
		// or the agent layer re-estimates a number the provider already gave us.
		ThinkingTokensMeasured: true,
	}
	if ue.Usage != want {
		t.Fatalf("usage on the error = %+v, want %+v", ue.Usage, want)
	}
	if ue.Model != "gpt-test" {
		t.Fatalf("model = %q, want gpt-test", ue.Model)
	}
	// The failure must stay a failure — usage rides along, it does not soften the
	// classification the caller acts on.
	if !strings.Contains(err.Error(), "connection closed after the turn") {
		t.Fatalf("error text lost the CLI's reason: %v", err)
	}
}

// A codex turn that never reached the provider (no CODEX_HOME to run in) spent
// nothing, so it must NOT be dressed up as a billable failure.
func TestCodexPreRequestFailureCarriesNoUsage(t *testing.T) {
	c := &CodexCLI{binPath: "codex-does-not-matter"}
	missing := filepath.Join(t.TempDir(), "no-such-home")
	_, _, err := c.runAttempt(context.Background(), nil, "prompt", "gpt-test", Request{}, missing)
	if err == nil {
		t.Fatal("a missing CODEX_HOME must fail the attempt")
	}
	if ue, ok := UsageFromError(err); ok {
		t.Fatalf("a pre-request failure was billed: %+v", ue)
	}
}
