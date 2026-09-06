package trajectory

import "sort"

// Session activity bouts (_Docs/78 §16). A session's bar on the Rota canvas is
// its lifetime — createdAt → updatedAt — so a conversation held in a few short
// bursts hours apart still reads as one solid block: a 20-hour bar for three
// hours of work. The only durable record of WHEN the session was actually
// working is the transcript, so the bar is split along its message timestamps.
//
// This file is the pure half: timestamps in, drawn stretches out. The reader
// that collects the timestamps lives in the API layer, the clipping and the
// live-tail rule in the canvas layout — both because a bout is a drawing fact,
// not a stored one. Nothing here is persisted; the graph is untouched.

// Span is one activity stretch in unix seconds. Start == End is a legitimate
// span: a single message that no neighbour joined.
type Span struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// DefaultBoutGapSec is the idle stretch that cuts one bout from the next. Ten
// minutes is short enough to separate two sittings and long enough to keep a
// think-then-answer pause inside a single bout.
const DefaultBoutGapSec int64 = 600

// MaxBouts caps how many stretches one session reports. A very long session
// with hundreds of sittings would draw a comb no reader can parse, and the
// payload grows with it; past the cap the oldest bouts are merged so the
// recent shape stays exact and the head degrades to a single block.
const MaxBouts = 64

// Bouts merges timestamps into activity stretches, cutting a new stretch
// whenever two consecutive timestamps are more than gapSec apart. The input
// need not be sorted; it is copied before sorting so the caller's slice is
// left alone. gapSec <= 0 falls back to DefaultBoutGapSec.
func Bouts(times []int64, gapSec int64) []Span {
	if len(times) == 0 {
		return nil
	}
	if gapSec <= 0 {
		gapSec = DefaultBoutGapSec
	}
	ts := make([]int64, len(times))
	copy(ts, times)
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })

	spans := []Span{{Start: ts[0], End: ts[0]}}
	for _, t := range ts[1:] {
		last := &spans[len(spans)-1]
		if t-last.End > gapSec {
			spans = append(spans, Span{Start: t, End: t})
			continue
		}
		last.End = t
	}
	return capBouts(spans)
}

// capBouts folds the oldest stretches into one so the result never exceeds
// MaxBouts. The merged head keeps its true start and end, so the timeline
// stays honest — it only loses the detail of its internal gaps.
func capBouts(spans []Span) []Span {
	if len(spans) <= MaxBouts {
		return spans
	}
	fold := len(spans) - MaxBouts + 1
	head := Span{Start: spans[0].Start, End: spans[fold-1].End}
	out := make([]Span, 0, MaxBouts)
	out = append(out, head)
	return append(out, spans[fold:]...)
}
