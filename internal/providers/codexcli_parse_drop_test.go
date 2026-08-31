package providers

import (
	"strings"
	"testing"
)

// A codex schema change used to manifest as missing tool steps or a truncated
// answer with zero diagnostic anywhere — no log, no trace step, no counter.
func TestCodexParserReportsMalformedLineOnceAndCountsTheRest(t *testing.T) {
	p := newCodexParser("", nil)
	p.feed(`{"type":"item.completed"`) // unterminated
	p.feed(`{"type":"item.completed"`)
	p.feed(`{"type":"item.completed"`)
	p.feed(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`)

	if _, err := p.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	var detailed, summary int
	for _, s := range p.resp.Trace {
		switch {
		case strings.Contains(s.Text, "+2 more line(s) dropped"):
			summary++
		case strings.Contains(s.Text, "[codex parse drop] line bytes="):
			detailed++
			if len(s.Text) > 500 {
				t.Fatalf("parse-drop note is %d bytes, want at most 500", len(s.Text))
			}
		}
	}
	if detailed != 1 || summary != 1 {
		t.Fatalf("want 1 detailed note + 1 summary, got %d/%d; trace=%+v", detailed, summary, p.resp.Trace)
	}
	if p.parseDropCount != 3 {
		t.Fatalf("parseDropCount = %d, want 3", p.parseDropCount)
	}
}

// A non-JSON line (codex prints plain text on some paths) is not a schema break
// and must stay silent, exactly as before.
func TestCodexParserIgnoresNonJSONLines(t *testing.T) {
	p := newCodexParser("", nil)
	p.feed("waiting for model...")
	p.feed("")
	if p.parseDropCount != 0 || len(p.resp.Trace) != 0 {
		t.Fatalf("plain-text lines were reported as drops: count=%d trace=%+v", p.parseDropCount, p.resp.Trace)
	}
}
