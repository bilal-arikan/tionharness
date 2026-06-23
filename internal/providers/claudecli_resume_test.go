package providers

import "testing"

// TestCLIParserCapturesSessionID verifies the stream parser records the CLI
// session id from the init event and lets the final result event's id override
// it (the result id is the one to resume from next turn).
func TestCLIParserCapturesSessionID(t *testing.T) {
	p := newCLIParser("claude-opus-4-8", nil)
	// system/init carries the initial session id.
	p.feed(`{"type":"system","subtype":"init","session_id":"sess-init"}`)
	// an assistant message with the answer.
	p.feed(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`)
	// the result event carries the (possibly rotated) final session id.
	p.feed(`{"type":"result","result":"hi","session_id":"sess-final"}`)

	out, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if out.SessionID != "sess-final" {
		t.Fatalf("session id: got %q, want sess-final (result overrides init)", out.SessionID)
	}
	if out.Text != "hi" {
		t.Fatalf("text: got %q, want hi", out.Text)
	}
}

// TestCLIParserSessionIDInitOnly verifies the init id is kept when no result id
// is present (defensive — the result envelope should normally carry it).
func TestCLIParserSessionIDInitOnly(t *testing.T) {
	p := newCLIParser("m", nil)
	p.feed(`{"type":"system","subtype":"init","session_id":"only-init"}`)
	p.feed(`{"type":"result","result":"ok"}`)
	out, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if out.SessionID != "only-init" {
		t.Fatalf("session id: got %q, want only-init", out.SessionID)
	}
}
