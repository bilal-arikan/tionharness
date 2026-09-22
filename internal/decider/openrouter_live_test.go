package decider

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// liveSource resolves every instance id to one real OpenRouter key.
type liveSource struct{ key string }

func (s liveSource) Endpoint(id string) (Endpoint, error) {
	return Endpoint{InstanceID: id, Kind: "openrouter", Authorize: func(h http.Header) { h.Set("Authorization", "Bearer "+s.key) }}, nil
}

func (liveSource) Instances() []InstanceInfo {
	return []InstanceInfo{{ID: "live", Kind: "openrouter", Enabled: true, Available: true}}
}

func (liveSource) Generation() uint64 { return 1 }

// TestLiveOpenRouterDecisions is a REAL call to OpenRouter's Decisions API
// (TypeSafe Jev) through the Hub — client, request shape, state preparation
// and answer mapping against the live service. It spends real (tiny) money, so
// it is gated behind OPENROUTER_LIVE_KEY and skipped in normal runs.
//
//	OPENROUTER_LIVE_KEY=sk-or-... go test ./internal/decider/ -run TestLiveOpenRouterDecisions -v
func TestLiveOpenRouterDecisions(t *testing.T) {
	key := os.Getenv("OPENROUTER_LIVE_KEY")
	if key == "" {
		t.Skip("set OPENROUTER_LIVE_KEY=sk-or-... to run the live Decisions API test")
	}
	h := NewHub(HubOptions{Source: liveSource{key: key}})
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.TimeoutMs = 10000
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := h.Decide(ctx, SiteToolRisk, Request{
		// The key-shaped token below must reach the service masked.
		State: "Tool call about to run in a shared repository: `git push --force origin main` (OPENROUTER_API_KEY=sk-or-v1-0000aaaa1111bbbb2222cccc)",
		Questions: map[string]Question{
			"needs_approval": Noul("Should a human approve this tool call before it runs?",
				"The call is destructive, irreversible or affects shared or remote state.",
				"The call only reads data or makes a local, easily reversible change."),
			"risk": Score("How risky is the tool call?", "Read-only", "Local reversible change", "Destructive or outward-facing"),
			"tier": Choice("Which model tier is the cheapest one that can handle a follow-up fix?", map[string]string{
				"fast": "Lookups and small localized edits.", "balanced": "Multi-file changes with a clear scope.", "frontier": "Architecture or high-stakes work.",
			}),
		},
	})
	if err != nil {
		t.Fatalf("live decision failed: %v", err)
	}
	a := resp.Answers["needs_approval"]
	if a.Type != QuestionNoul || a.Probability < 0 || a.Probability > 1 {
		t.Errorf("noul answer = %+v", a)
	}
	if tier := resp.Answers["tier"]; tier.Choice != "fast" && tier.Choice != "balanced" && tier.Choice != "frontier" {
		t.Errorf("choice answer = %+v", tier)
	}
	if r := resp.Answers["risk"]; r.Level() < 0 || r.Level() > 2 {
		t.Errorf("score answer = %+v", r)
	}
	if resp.Usage.InputTokens == 0 || !strings.HasPrefix(resp.ServedModel, "typesafe/jev") || resp.Model != JevModel {
		t.Errorf("accounting = %+v served=%q model=%q", resp.Usage, resp.ServedModel, resp.Model)
	}
	t.Logf("served=%s latency=%dms input=%d cost=$%.7f needs_approval=%.2f risk=%v tier=%s(%.2f)",
		resp.ServedModel, resp.LatencyMs, resp.Usage.InputTokens, resp.Usage.CostUSD,
		a.Probability, resp.Answers["risk"].Score, resp.Answers["tier"].Choice, resp.Answers["tier"].Strength())

	// The pinned alias resolves too.
	if _, err := h.Decide(ctx, SiteToolRisk, Request{Model: JevLatestModel, State: "hello", Questions: map[string]Question{
		"greeting": Noul("Is this message a greeting?", "", ""),
	}}); err != nil {
		t.Errorf("latest alias: %v", err)
	}

	// A half-specified noul pair is completed by Normalized, so the service
	// accepts it (it answers 400 to the raw shape).
	if _, err := h.Decide(ctx, SiteToolRisk, Request{State: "hello there", Questions: map[string]Question{
		"greeting": Noul("Is this message a greeting?", "The message greets someone.", ""),
	}}); err != nil {
		t.Errorf("half-specified noul: %v", err)
	}
}
