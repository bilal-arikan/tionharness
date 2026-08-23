package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIParserReportsMalformedJSONOnceAndContinues(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"assistant","message":{"content":[{"type":"text","text":"before"}]}}`)
	p.feed(`{"type":"assistant"`)
	p.feed(`{"type":"result"`)
	p.feed(`{"type":"assistant","message":{"content":[{"type":"text","text":" after"}]}}`)

	var notes int
	for _, step := range p.resp.Trace {
		if strings.Contains(step.Text, "[claude-cli parse drop]") {
			notes++
			if len(step.Text) > 500 {
				t.Fatalf("parse-drop note is %d bytes, want at most 500", len(step.Text))
			}
			if !strings.Contains(step.Text, "line bytes=") || !strings.Contains(step.Text, "unexpected end of JSON input") {
				t.Fatalf("parse-drop note lacks length or error: %q", step.Text)
			}
		}
	}
	if notes != 1 {
		t.Fatalf("parse-drop notes = %d, want 1; trace=%+v", notes, p.resp.Trace)
	}
	p.flushText()
	if got := p.resp.Trace[len(p.resp.Trace)-1].Text; got != "after" {
		t.Fatalf("parser did not continue after malformed JSON: final text step = %q", got)
	}
}

func TestCLIParserAcceptsEventLargerThanOneMiB(t *testing.T) {
	largeInput := map[string]string{"content": strings.Repeat("line one\nline two\n", 70_000)}
	input, err := json.Marshal(largeInput)
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(cliEvent{
		Type: "assistant",
		Message: &cliMessage{Content: []cliBlock{{
			Type:  "tool_use",
			ID:    "large-tool",
			Name:  "Write",
			Input: input,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(event) <= 1<<20 {
		t.Fatalf("fixture is %d bytes, want more than 1 MiB", len(event))
	}

	p := newCLIParser("", nil)
	p.feed(string(event))
	if len(p.resp.Trace) != 1 || p.resp.Trace[0].Tool != "Write" {
		t.Fatalf("large event was not parsed as a tool call: %+v", p.resp.Trace)
	}
	if p.notedParseDrop {
		t.Fatal("valid large event was reported as a parse drop")
	}
}
