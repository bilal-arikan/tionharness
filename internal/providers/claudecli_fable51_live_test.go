package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveClaudeCLIFable51 drives the claude-cli provider against the REAL CLI
// with the Claude Fable 5.1 model id, through the same request shaping the
// runtime uses (appended system prompt, dynamic tail, the --tools built-in
// allowlist). It spends real subscription quota, so it is gated behind
// TIONHARNESS_LIVE_CLI_FABLE51=1. The CLI's own login is used (CLAUDE_CONFIG_DIR
// unset → ~/.claude), because the app's claude-home may be logged out.
//
//	TIONHARNESS_LIVE_CLI_FABLE51=1 go test ./internal/providers/ -run TestLiveClaudeCLIFable51 -v
func TestLiveClaudeCLIFable51(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_CLI_FABLE51") != "1" {
		t.Skip("set TIONHARNESS_LIVE_CLI_FABLE51=1 to run the live claude-cli Fable 5.1 test")
	}
	bin := "claude"
	if p := os.Getenv("TIONHARNESS_CLAUDE_BIN"); p != "" {
		bin = p
	}
	const model = "claude-fable-5-1"
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	cases := []struct {
		name     string
		restrict bool
		tools    []string
	}{
		{"no-builtins (auxiliary shape)", true, nil},
		{"worker allowlist", true, []string{"Read", "Grep", "Glob", "ToolSearch"}},
		{"unrestricted", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClaudeCLI(bin, model, "", "", "")
			resp, err := c.Complete(ctx, Request{
				Model:                  model,
				System:                 "You are inside an automated test. Follow the instruction literally.",
				SystemDynamic:          "Current date (turn start): 2026-09-03",
				Messages:               []Message{{Role: RoleUser, Text: "Reply with exactly the word: ok"}},
				MaxTokens:              256,
				CLIRestrictNativeTools: tc.restrict,
				CLINativeTools:         tc.tools,
				NativeWebSearch:        true,
			})
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}
			t.Logf("model=%s stop=%s text=%q in=%d out=%d cacheW=%d cacheR=%d calls=%d",
				resp.Model, resp.StopReason, strings.TrimSpace(resp.Text),
				resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CacheWriteTokens, resp.Usage.CacheReadTokens, resp.ProviderCalls)
			if !strings.Contains(resp.Model, "fable-5-1") {
				t.Errorf("served model = %q, want claude-fable-5-1", resp.Model)
			}
			if !strings.Contains(strings.ToLower(resp.Text), "ok") {
				t.Errorf("text = %q, want ok", resp.Text)
			}
			if resp.Usage.InputTokens+resp.Usage.CacheWriteTokens+resp.Usage.CacheReadTokens == 0 {
				t.Errorf("no usage reported")
			}
		})
	}
}
