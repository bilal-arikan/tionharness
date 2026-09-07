package providers

import "testing"

// The first assistant message's usage is the prompt exactly as composed; later
// tool-loop round-trips carry a grown prompt and the result envelope is the
// cumulative sum. Only the first is the calibration ground truth.
func TestCLIParserKeepsFirstCallPromptTokens(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"a"}],` +
		`"usage":{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":20000,"cache_creation_input_tokens":500}}}`)
	p.feed(`{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"b"}],` +
		`"usage":{"input_tokens":900,"output_tokens":4,"cache_read_input_tokens":20510,"cache_creation_input_tokens":0}}}`)
	p.feed(`{"type":"result","is_error":false,"result":"ab","num_turns":2,` +
		`"usage":{"input_tokens":910,"output_tokens":7,"cache_read_input_tokens":40510,"cache_creation_input_tokens":500}}`)

	if got := p.resp.FirstCallPromptTokens; got != 20510 {
		t.Fatalf("FirstCallPromptTokens = %d, want 20510 (10+20000+500 of the first message)", got)
	}
	if p.resp.ProviderCalls != 2 {
		t.Fatalf("ProviderCalls = %d, want 2", p.resp.ProviderCalls)
	}
}

func TestCLIParserFirstCallPromptTokensZeroWithoutUsage(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"a"}]}}`)
	p.feed(`{"type":"result","is_error":false,"result":"a","num_turns":1}`)
	if p.resp.FirstCallPromptTokens != 0 {
		t.Fatalf("FirstCallPromptTokens = %d, want 0 with no per-message usage", p.resp.FirstCallPromptTokens)
	}
}
