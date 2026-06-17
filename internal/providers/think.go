package providers

import "strings"

// Some OpenAI-compatible models (notably MiniMax) embed their chain-of-thought
// in the visible content wrapped in <think>…</think> tags. thinkFilter splits a
// (possibly chunked) text stream into visible text and reasoning, tolerating
// tags that straddle chunk boundaries, so the reasoning can be routed to the UI
// thinking block instead of polluting the answer.
type thinkFilter struct {
	inside  bool
	pending string // buffered tail that may be the start of a tag
}

const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

// tagPrefix reports whether s is a (proper or full) prefix of either tag, i.e.
// it might still grow into a tag once more input arrives.
func tagPrefix(s string) bool {
	return strings.HasPrefix(thinkOpenTag, s) || strings.HasPrefix(thinkCloseTag, s)
}

// feed consumes a chunk and returns any newly-resolved visible text and
// reasoning text. Bytes whose classification is still ambiguous (a partial tag
// at the end) are held in pending until the next feed or flush.
func (f *thinkFilter) feed(chunk string) (text, think string) {
	f.pending += chunk
	var tb, kb strings.Builder
	for f.pending != "" {
		want := thinkOpenTag
		if f.inside {
			want = thinkCloseTag
		}
		i := strings.IndexByte(f.pending, '<')
		if i < 0 {
			// No tag start at all — the whole buffer is plain content.
			f.write(&tb, &kb, f.pending)
			f.pending = ""
			break
		}
		// Emit everything before the '<' as content.
		f.write(&tb, &kb, f.pending[:i])
		rest := f.pending[i:]
		if strings.HasPrefix(rest, want) {
			f.inside = !f.inside
			f.pending = rest[len(want):]
			continue
		}
		if tagPrefix(rest) {
			// Possibly the start of a tag split across chunks — wait for more.
			f.pending = rest
			break
		}
		// A literal '<' that is not our tag: emit it and move on.
		f.write(&tb, &kb, "<")
		f.pending = rest[1:]
	}
	return tb.String(), kb.String()
}

// flush returns any buffered remainder at end of stream, classified by the
// current state (unterminated reasoning -> think, otherwise -> text).
func (f *thinkFilter) flush() (text, think string) {
	if f.pending == "" {
		return "", ""
	}
	p := f.pending
	f.pending = ""
	if f.inside {
		return "", p
	}
	return p, ""
}

func (f *thinkFilter) write(tb, kb *strings.Builder, s string) {
	if s == "" {
		return
	}
	if f.inside {
		kb.WriteString(s)
	} else {
		tb.WriteString(s)
	}
}

// splitThink runs a whole string through the filter, returning the visible text
// (think tags removed) and the concatenated reasoning. Used by the non-stream
// Complete path.
func splitThink(s string) (text, think string) {
	f := &thinkFilter{}
	t1, k1 := f.feed(s)
	t2, k2 := f.flush()
	return strings.TrimSpace(t1 + t2), strings.TrimSpace(k1 + k2)
}
