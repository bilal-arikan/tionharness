package providers

import "testing"

func TestSplitThink(t *testing.T) {
	cases := []struct {
		in, text, think string
	}{
		{"<think>reasoning</think>\n\nHello", "Hello", "reasoning"},
		{"no tags here", "no tags here", ""},
		{"<think>a</think>answer<think>b</think>", "answer", "ab"},
		{"plain <not a tag> text", "plain <not a tag> text", ""},
	}
	for _, c := range cases {
		text, think := splitThink(c.in)
		if text != c.text || think != c.think {
			t.Errorf("splitThink(%q) = (%q,%q), want (%q,%q)", c.in, text, think, c.text, c.think)
		}
	}
}

// TestThinkFilterChunked verifies tags split across feed() boundaries are
// handled — the streaming case where a tag straddles SSE chunks.
func TestThinkFilterChunked(t *testing.T) {
	f := &thinkFilter{}
	var text, think string
	for _, chunk := range []string{"Hel", "lo <th", "ink>rea", "son</thi", "nk> world"} {
		tx, th := f.feed(chunk)
		text += tx
		think += th
	}
	tx, th := f.flush()
	text += tx
	think += th
	if text != "Hello  world" {
		t.Errorf("text = %q, want %q", text, "Hello  world")
	}
	if think != "reason" {
		t.Errorf("think = %q, want %q", think, "reason")
	}
}
