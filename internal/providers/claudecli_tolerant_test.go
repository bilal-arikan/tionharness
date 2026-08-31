package providers

import (
	"strings"
	"testing"
)

// A result envelope whose api_error_status is a NUMBER (an HTTP status) used to
// fail strict decoding and take the WHOLE envelope with it: no session_id (so
// --resume broke), no usage (the turn billed as zero) and no sawResult (a turn
// the CLI completed reported as "no result in stream").
func TestCLIParserKeepsResultEnvelopeWithNumericAPIErrorStatus(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","is_error":true,"api_error_status":429,"stop_reason":"stop_sequence",` +
		`"session_id":"sess-42","result":"upstream refused","num_turns":3,` +
		`"usage":{"input_tokens":1200,"output_tokens":7,"cache_read_input_tokens":900}}`)

	if !p.sawResult {
		t.Fatal("result envelope was dropped: sawResult is false")
	}
	if p.resp.SessionID != "sess-42" {
		t.Fatalf("session id = %q, want sess-42", p.resp.SessionID)
	}
	if !p.hadError {
		t.Fatal("is_error was lost: hadError is false")
	}
	if !p.rateLimited {
		t.Fatal("numeric api_error_status 429 was not classified as a rate limit")
	}
	if p.resp.Usage.InputTokens != 1200 || p.resp.Usage.OutputTokens != 7 || p.resp.Usage.CacheReadTokens != 900 {
		t.Fatalf("usage lost on the error envelope: %+v", p.resp.Usage)
	}
	if p.resp.ProviderCalls != 3 {
		t.Fatalf("num_turns = %d, want 3", p.resp.ProviderCalls)
	}
	if p.notedParseDrop || p.notedFieldDrop {
		t.Fatalf("a number in api_error_status must decode cleanly, not as a drop; trace=%+v", p.resp.Trace)
	}
}

// A field whose shape the parser genuinely cannot model (an object where a
// string is declared) must cost only that field — the rest of the event is kept
// and the loss is reported, never swallowed.
func TestCLIParserSalvagesEventAndReportsDroppedField(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","session_id":"sess-7","result":{"unexpected":"object"},` +
		`"usage":{"input_tokens":11,"output_tokens":2}}`)

	if !p.sawResult {
		t.Fatal("event was dropped entirely instead of salvaged")
	}
	if p.resp.SessionID != "sess-7" {
		t.Fatalf("session id = %q, want sess-7", p.resp.SessionID)
	}
	if p.resp.Usage.InputTokens != 11 {
		t.Fatalf("usage lost during salvage: %+v", p.resp.Usage)
	}
	var note string
	for _, s := range p.resp.Trace {
		if strings.Contains(s.Text, "[claude-cli field drop]") {
			note = s.Text
		}
	}
	if note == "" {
		t.Fatalf("salvage was silent; trace=%+v", p.resp.Trace)
	}
	if !strings.Contains(note, "result") {
		t.Fatalf("field-drop note does not name the lost field: %q", note)
	}
}

func TestCLIParserSummarizesRepeatedParseDrops(t *testing.T) {
	p := newCLIParser("", nil)
	for i := 0; i < 4; i++ {
		p.feed(`{"type":"assistant"`) // unterminated → not salvageable
	}
	p.feed(`{"type":"result","result":"done","session_id":"s"}`)
	if _, err := p.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	var detailed, summary int
	for _, s := range p.resp.Trace {
		switch {
		case strings.Contains(s.Text, "+3 more event(s) dropped"):
			summary++
		case strings.Contains(s.Text, "[claude-cli parse drop] line bytes="):
			detailed++
		}
	}
	if detailed != 1 || summary != 1 {
		t.Fatalf("want 1 detailed note + 1 summary, got %d/%d; trace=%+v", detailed, summary, p.resp.Trace)
	}
}

// Rate-limit and auth failures are exactly the turns whose input tokens were
// still paid for, so the result envelope's usage must survive the error branch.
func TestCLIParserRecordsUsageOnErrorResult(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","is_error":true,"result":"Not logged in · Please run /login",` +
		`"num_turns":2,"usage":{"input_tokens":4321,"output_tokens":0,"cache_creation_input_tokens":120}}`)

	if !p.notLoggedIn {
		t.Fatal("auth failure was not classified")
	}
	if p.resp.Usage.InputTokens != 4321 || p.resp.Usage.CacheWriteTokens != 120 {
		t.Fatalf("usage discarded on the error path: %+v", p.resp.Usage)
	}
	if p.resp.ProviderCalls != 2 {
		t.Fatalf("num_turns = %d, want 2", p.resp.ProviderCalls)
	}
}
