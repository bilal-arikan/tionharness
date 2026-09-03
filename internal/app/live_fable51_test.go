package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/config"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
)

// TestLiveFable51 is a REAL end-to-end check of the Claude Fable 5.1 path against
// the live API, using the Anthropic key stored (encrypted) in this machine's
// TionHarness settings — the key never leaves the process. It spends real tokens
// (a few cents), so it is gated behind TIONHARNESS_LIVE_FABLE51=1 and skipped in
// normal runs.
//
// Three rounds mirror what the native tool loop does inside one turn:
//
//  1. opening request with a tool → expects a tool_use and a verbatim RawContent
//
//  2. append-only replay (assistant RawContent + tool_result) → the thinking
//     block must be accepted: no input_transformations
//
//  3. the SAME replay with the opening user text edited — the preserved-thinking
//     check must fire; with the drop_block policy the request still succeeds
//     and input_transformations names the dropped block (prefix_binding_mismatch)
//
//     TIONHARNESS_LIVE_FABLE51=1 go test ./internal/app/ -run TestLiveFable51 -v
func TestLiveFable51(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_FABLE51") != "1" {
		t.Skip("set TIONHARNESS_LIVE_FABLE51=1 to run the live Fable 5.1 check")
	}
	dataDir := config.DefaultDataDir()
	secret, err := config.LoadSecret(dataDir)
	if err != nil {
		t.Fatalf("load secret: %v", err)
	}
	store, err := settings.Open(dataDir, secret)
	if err != nil {
		t.Fatalf("open settings: %v", err)
	}
	key := store.AnthropicKey()
	if key == "" {
		t.Skip("no Anthropic API key configured in settings")
	}

	const model = "claude-fable-5-1"
	a := providers.NewAnthropic(key).WithBetas(true, false, false).WithRefusalFallback(true)
	ctx := context.Background()
	price, _ := providers.PriceFor("anthropic", model)
	var total providers.Usage
	report := func(label string, resp *providers.Response) {
		u := resp.Usage
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.CacheReadTokens += u.CacheReadTokens
		total.CacheWriteTokens += u.CacheWriteTokens
		t.Logf("%s: model=%s stop=%s in=%d out=%d cacheRead=%d cacheWrite=%d transformations=%+v",
			label, resp.Model, resp.StopReason, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, resp.InputTransformations)
	}

	tool := providers.ToolDef{
		Name:        "get_time",
		Description: "Returns the current wall-clock time for an IANA time zone.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"zone":{"type":"string","description":"IANA zone, e.g. Europe/Istanbul"}},"required":["zone"],"additionalProperties":false}`),
	}
	opening := "Use the get_time tool for the zone Europe/Istanbul, then reply with exactly the word done."
	dynamic := "Current date (turn start): 2026-09-03 15:00 (+03:00)"
	base := providers.Request{
		Model:         model,
		System:        "You are a terse assistant inside an automated test. Follow the instruction literally.",
		SystemDynamic: dynamic,
		Tools:         []providers.ToolDef{tool},
		MaxTokens:     2048,
	}

	// Round 1: opening request.
	r1 := base
	r1.Messages = []providers.Message{{Role: providers.RoleUser, Text: opening}}
	resp1, err := a.Complete(ctx, r1)
	if err != nil {
		t.Fatalf("round 1: %v", err)
	}
	report("round1", resp1)
	if resp1.StopReason == providers.StopRefusal {
		t.Fatalf("round 1 refused: %+v", resp1.StopDetails)
	}
	if len(resp1.ToolCalls) == 0 || len(resp1.RawContent) == 0 {
		t.Fatalf("round 1: expected a tool_use with verbatim RawContent, got calls=%d raw=%dB text=%q", len(resp1.ToolCalls), len(resp1.RawContent), resp1.Text)
	}
	if !strings.Contains(string(resp1.RawContent), `"thinking"`) {
		t.Logf("round 1: RawContent carries no thinking block (display omitted returns empty thinking; fine)")
	}
	if len(resp1.InputTransformations) != 0 {
		t.Errorf("round 1: fresh request must not report dropped blocks: %+v", resp1.InputTransformations)
	}

	// Round 2: append-only replay of the same turn.
	toolResult := providers.Message{Role: providers.RoleUser, ToolResults: []providers.ToolResult{{CallID: resp1.ToolCalls[0].ID, Content: "15:00 Europe/Istanbul"}}}
	r2 := base
	r2.Messages = []providers.Message{
		{Role: providers.RoleUser, Text: opening},
		{Role: providers.RoleAssistant, RawContent: resp1.RawContent},
		toolResult,
	}
	resp2, err := a.Complete(ctx, r2)
	if err != nil {
		t.Fatalf("round 2 (append-only replay): %v", err)
	}
	report("round2", resp2)
	if len(resp2.InputTransformations) != 0 {
		t.Errorf("round 2: append-only replay must keep the thinking block, got %+v", resp2.InputTransformations)
	}
	if !strings.Contains(strings.ToLower(resp2.Text), "done") {
		t.Logf("round 2 text = %q (expected to contain 'done')", resp2.Text)
	}

	// Round 3: the same replay with the opening user text edited — a history edit.
	r3 := base
	r3.Messages = []providers.Message{
		{Role: providers.RoleUser, Text: opening + " (edited)"},
		{Role: providers.RoleAssistant, RawContent: resp1.RawContent},
		toolResult,
	}
	resp3, err := a.Complete(ctx, r3)
	if err != nil {
		t.Fatalf("round 3 (edited history): expected drop_block to rescue the request, got error: %v", err)
	}
	report("round3-edited", resp3)
	if len(resp3.InputTransformations) == 0 {
		t.Logf("round 3: no dropped block reported — this organization does not enforce the prefix check even when opted in, or the block signature tolerated the edit")
	} else {
		for _, tr := range resp3.InputTransformations {
			t.Logf("round 3 dropped: %s at %s (%s)", tr.Type, tr.Path, tr.Reason)
		}
	}

	if price.InputPerMTok > 0 {
		t.Logf("total: in=%d out=%d cacheRead=%d cacheWrite=%d ≈ $%.4f",
			total.InputTokens, total.OutputTokens, total.CacheReadTokens, total.CacheWriteTokens,
			price.CostDetailed(total.InputTokens, total.OutputTokens, total.CacheReadTokens, total.CacheWriteTokens))
	}
}
