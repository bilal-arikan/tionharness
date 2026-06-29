package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveAntigravityCLI is a REAL end-to-end check of the antigravity-cli
// provider: it spends actual quota and requires the agy CLI to be authenticated
// (interactive Google Sign-In on first run, or ANTIGRAVITY_API_KEY), so it is
// gated behind SWARMGO_LIVE_AGY=1 and skipped in normal runs. It proves the
// headless --print path returns a parsed plain-text answer.
//
//	$env:SWARMGO_LIVE_AGY="1"   # (after `agy` sign-in)
//	go test ./internal/providers/ -run TestLiveAntigravityCLI -v
func TestLiveAntigravityCLI(t *testing.T) {
	if os.Getenv("SWARMGO_LIVE_AGY") != "1" {
		t.Skip("set SWARMGO_LIVE_AGY=1 to run the live antigravity-cli test")
	}
	bin := "agy"
	if p := os.Getenv("SWARMGO_AGY_BIN"); p != "" {
		bin = p
	}
	// "" model → let agy auto-select; "" apiKey → inherit ambient ANTIGRAVITY_API_KEY.
	a := NewAntigravityCLI(bin, os.Getenv("SWARMGO_AGY_MODEL"), os.Getenv("ANTIGRAVITY_API_KEY"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	resp, err := a.Complete(ctx, Request{
		PermissionMode: "auto",
		System:         "You are a calculator. Answer only with the number, nothing else.",
		Messages: []Message{{
			Role: RoleUser,
			Text: "What is 17 plus 25?",
		}},
	})
	if err != nil {
		t.Fatalf("antigravity Complete failed: %v", err)
	}
	t.Logf("text=%q model=%q", resp.Text, resp.Model)
	if strings.TrimSpace(resp.Text) == "" {
		t.Fatalf("empty answer text")
	}
	if !strings.Contains(resp.Text, "42") {
		t.Fatalf("expected the answer to contain 42, got %q", resp.Text)
	}
}
