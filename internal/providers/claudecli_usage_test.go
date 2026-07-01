package providers

import "testing"

// TestCLIParserNumTurns checks the claude-cli parser captures the result event's
// num_turns into Response.ProviderCalls and sums per-call cache usage cumulatively,
// so a downstream consumer can divide to recover the per-call (single-pass) context.
// Numbers mirror a real captured stream: two internal round-trips whose cacheReads
// (21628 + 25030) sum to the result envelope's 46658.
func TestCLIParserNumTurns(t *testing.T) {
	p := newCLIParser("claude-haiku-4-5", nil)
	lines := []string{
		`{"type":"assistant","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":21628,"cache_creation_input_tokens":7309},"content":[{"type":"text","text":"ok"}]}}`,
		`{"type":"assistant","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":6,"output_tokens":1,"cache_read_input_tokens":25030,"cache_creation_input_tokens":7410},"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"done","num_turns":2,"session_id":"sess-1","usage":{"input_tokens":8,"output_tokens":118,"cache_read_input_tokens":46658,"cache_creation_input_tokens":14719}}`,
	}
	for _, l := range lines {
		p.feed(l)
	}
	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.ProviderCalls != 2 {
		t.Fatalf("ProviderCalls = %d, want 2 (num_turns)", resp.ProviderCalls)
	}
	// The result envelope is authoritative for the turn aggregate (cumulative).
	if resp.Usage.CacheReadTokens != 46658 {
		t.Fatalf("CacheReadTokens = %d, want 46658 (cumulative)", resp.Usage.CacheReadTokens)
	}
	if resp.Usage.CacheWriteTokens != 14719 {
		t.Fatalf("CacheWriteTokens = %d, want 14719", resp.Usage.CacheWriteTokens)
	}
	// Dividing the cumulative turn by ProviderCalls recovers the per-call context,
	// which must land near the real single-pass sizes (28939 and 32446) — NOT the
	// inflated cumulative (61385) the old preview reported.
	perCall := (resp.Usage.InputTokens + resp.Usage.CacheReadTokens + resp.Usage.CacheWriteTokens) / resp.ProviderCalls
	if perCall < 25000 || perCall > 35000 {
		t.Fatalf("per-call context = %d, want ~30k (between the two real single-pass sizes)", perCall)
	}
}
