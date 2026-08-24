package conversation

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// RepairNote records one applied sequence repair, for the debug journal and
// logs — repairs are never silent.
type RepairNote struct {
	Rule   string // machine tag: orphan_tool_result / duplicate_tool_result / missing_tool_result / merged_assistant_tool_calls
	Detail string // human-readable specifics (tool call ids involved)
}

// repairCancelledMsg is the synthetic tool_result content for a tool_use that
// lost its answer (crash, interrupted turn, upstream bug). Mirrors the tool
// loop's cancellation message so the model reads a consistent vocabulary.
const repairCancelledMsg = "tool call cancelled: no result was recorded for this tool call (repaired)"

// RepairSequence enforces the provider tool-pairing invariants on an in-flight
// message slice: every assistant tool_use is answered by exactly one tool_result
// in the immediately following user message, and no tool_result exists without
// its tool_use. Violations (from crashes, interrupted turns, or upstream bugs)
// otherwise surface as provider 400s that kill the whole turn.
//
// Repairs, in order per message:
//  1. duplicate tool_result (same call id) → keep the first, drop the rest;
//  2. orphan tool_result (no matching tool_use in the preceding assistant
//     message) → drop it (drop the whole message if nothing else remains);
//  3. assistant tool_use with no answering user message → merge with a directly
//     following assistant tool_use message when that is safe (neither side
//     carries provider-native RawContent), else synthesize cancelled
//     tool_results so the pair invariant holds;
//  4. a dangling trailing assistant tool_use → synthesize cancelled results.
//
// Pure and idempotent: a well-formed slice is returned unchanged (same backing
// array, nil notes), so callers may run it every iteration for free.
//
// "For free" is load-bearing and enforced by BenchmarkRepairSequence: the tool
// loop calls this before EVERY provider call, over the whole in-flight history,
// so the untouched path must not allocate. It used to — a full Message slice up
// front plus a set per message, ~1.3 MB per pass on a 10k-message history, all
// discarded — which is exactly the cost a "free" contract promises away.
func RepairSequence(msgs []providers.Message) ([]providers.Message, []RepairNote) {
	var notes []RepairNote
	out := msgs
	changed := false
	// ensure copies on first mutation so the caller's slice is never half-mutated.
	mutate := func() {
		if !changed {
			out = append([]providers.Message(nil), msgs...)
			changed = true
		}
	}

	// Pass 1: dedupe duplicate tool_results within each user message.
	for i := range msgs {
		m := &msgs[i]
		if m.Role != providers.RoleUser || len(m.ToolResults) < 2 {
			continue
		}
		seen := make(map[string]bool, len(m.ToolResults))
		kept := m.ToolResults[:0:0]
		for _, tr := range m.ToolResults {
			if tr.CallID != "" && seen[tr.CallID] {
				notes = append(notes, RepairNote{Rule: "duplicate_tool_result", Detail: "call " + tr.CallID})
				continue
			}
			seen[tr.CallID] = true
			kept = append(kept, tr)
		}
		if len(kept) != len(m.ToolResults) {
			mutate()
			out[i].ToolResults = kept
		}
	}

	// Pass 2: walk pairs — validate each user result message against the
	// assistant tool_use message straight before it; heal unanswered tool_use.
	// Work on `out` (post-dedupe view); rebuild only when structure changes.
	// rebuilt is materialised ONLY once the sequence actually diverges. It used to
	// be allocated up front on every call — a full Message slice (~136 KB per 1000
	// messages) built and then thrown away at the bottom whenever `structural`
	// stayed false, which is the overwhelmingly common case: this runs before
	// EVERY provider call in the tool loop, on the whole in-flight history.
	var rebuilt []providers.Message
	structural := false
	// diverge switches to the rebuilt sequence, seeded with everything accepted so
	// far. Every branch that changes the sequence must call it before emitting.
	diverge := func(upto int) {
		if rebuilt == nil {
			rebuilt = make([]providers.Message, 0, len(out)+1)
			rebuilt = append(rebuilt, out[:upto]...)
		}
		structural = true
	}
	// emit records an accepted message. Before divergence out[:i+1] already IS the
	// answer, so there is nothing to copy.
	emit := func(msgs ...providers.Message) {
		if rebuilt != nil {
			rebuilt = append(rebuilt, msgs...)
		}
	}
	// lastEmitted is the message the previous iteration accepted — the tail of
	// rebuilt once it exists, otherwise simply out[i-1] (identical until divergence).
	lastEmitted := func(i int) (providers.Message, bool) {
		if rebuilt != nil {
			if len(rebuilt) == 0 {
				return providers.Message{}, false
			}
			return rebuilt[len(rebuilt)-1], true
		}
		if i == 0 {
			return providers.Message{}, false
		}
		return out[i-1], true
	}
	for i := 0; i < len(out); i++ {
		m := out[i]
		if m.Role == providers.RoleUser && len(m.ToolResults) > 0 {
			// Valid ids = calls of the immediately preceding assistant message
			// (the provider contract); anything else is an orphan.
			var validCalls []providers.ToolCall
			if prev, ok := lastEmitted(i); ok && prev.Role == providers.RoleAssistant {
				validCalls = prev.ToolCalls
			}
			// Scanned, not indexed into a set: a batch is a handful of calls, so
			// the linear check wins outright and — unlike a map — allocates nothing
			// on the hot path.
			orphan := false
			for _, tr := range m.ToolResults {
				if !hasCallID(validCalls, tr.CallID) {
					orphan = true
					break
				}
			}
			if !orphan {
				emit(m) // untouched: keep the results slice as-is, no copy
				continue
			}
			diverge(i)
			kept := make([]providers.ToolResult, 0, len(m.ToolResults))
			for _, tr := range m.ToolResults {
				if !hasCallID(validCalls, tr.CallID) {
					notes = append(notes, RepairNote{Rule: "orphan_tool_result", Detail: "call " + tr.CallID})
					continue
				}
				kept = append(kept, tr)
			}
			if len(kept) == 0 && m.Text == "" {
				continue // nothing left — drop the message entirely
			}
			m.ToolResults = kept
			emit(m)
			continue
		}
		if m.Role == providers.RoleAssistant && len(m.ToolCalls) > 0 {
			answered := i+1 < len(out) &&
				out[i+1].Role == providers.RoleUser && len(out[i+1].ToolResults) > 0
			if !answered {
				// A directly following assistant tool_use message means one batch
				// was split in two: merge when neither side carries RawContent
				// (provider-native blocks cannot be joined portably).
				if i+1 < len(out) && out[i+1].Role == providers.RoleAssistant &&
					len(out[i+1].ToolCalls) > 0 && m.RawContent == nil && out[i+1].RawContent == nil {
					next := out[i+1]
					merged := m
					merged.ToolCalls = append(append([]providers.ToolCall(nil), m.ToolCalls...), next.ToolCalls...)
					if next.Text != "" {
						if merged.Text != "" {
							merged.Text += "\n" + next.Text
						} else {
							merged.Text = next.Text
						}
					}
					notes = append(notes, RepairNote{
						Rule:   "merged_assistant_tool_calls",
						Detail: fmt.Sprintf("%d + %d calls", len(m.ToolCalls), len(next.ToolCalls)),
					})
					diverge(i)
					emit(merged)
					i++ // consumed the next message too
					// The merged batch is still unanswered; the next loop pass (below)
					// cannot revisit it, so synthesize its results here if the message
					// after the pair is not a result message.
					if i+1 >= len(out) || out[i+1].Role != providers.RoleUser || len(out[i+1].ToolResults) == 0 {
						emit(syntheticResults(merged.ToolCalls))
						notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized (post-merge)", len(merged.ToolCalls))})
					}
					continue
				}
				// No mergeable neighbour: answer the batch with synthetic
				// cancelled results so the pair invariant holds.
				notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized", len(m.ToolCalls))})
				diverge(i)
				emit(m, syntheticResults(m.ToolCalls))
				continue
			}
			// Answered — but only for the ids the next message actually carries;
			// synthesize the gap so partial results (mid-batch crash) still pair up.
			// Scanned rather than set-indexed, for the same reason as above: this
			// runs for every answered tool_use on every provider call, and the two
			// maps it used to build were the per-message allocation in the hot path.
			next := out[i+1]
			missing := 0
			for _, tc := range m.ToolCalls {
				if !hasResultID(next.ToolResults, tc.ID) {
					missing++
				}
			}
			if missing > 0 {
				// Rebuild the answer: keep only results matching this batch's ids
				// (anything else is an orphan that would 400 anyway), then fill
				// the gaps with synthetic cancelled results.
				diverge(i)
				results := make([]providers.ToolResult, 0, len(m.ToolCalls))
				for _, tr := range next.ToolResults {
					if hasCallID(m.ToolCalls, tr.CallID) {
						results = append(results, tr)
					} else {
						notes = append(notes, RepairNote{Rule: "orphan_tool_result", Detail: "call " + tr.CallID})
					}
				}
				for _, tc := range m.ToolCalls {
					if !hasResultID(next.ToolResults, tc.ID) {
						results = append(results, providers.ToolResult{CallID: tc.ID, Content: repairCancelledMsg, IsError: true})
					}
				}
				next.ToolResults = results
				notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized (partial batch)", missing)})
				emit(m, next)
				i++
				continue
			}
		}
		emit(m)
	}
	if structural {
		return rebuilt, notes
	}
	if changed {
		return out, notes
	}
	return msgs, notes
}

// syntheticResults builds the user message answering calls with cancelled
// tool_results, used when the real results were lost.
func syntheticResults(calls []providers.ToolCall) providers.Message {
	results := make([]providers.ToolResult, 0, len(calls))
	for _, tc := range calls {
		results = append(results, providers.ToolResult{CallID: tc.ID, Content: repairCancelledMsg, IsError: true})
	}
	return providers.Message{Role: providers.RoleUser, ToolResults: results}
}

// hasCallID reports whether the batch contains a call with this id. Linear on
// purpose: a tool batch is a handful of calls, so scanning beats building a set —
// and it allocates nothing, which matters because RepairSequence runs before
// every provider call over the whole in-flight history.
func hasCallID(calls []providers.ToolCall, id string) bool {
	for _, tc := range calls {
		if tc.ID == id {
			return true
		}
	}
	return false
}

// hasResultID is the mirror of hasCallID for the answering side.
func hasResultID(results []providers.ToolResult, id string) bool {
	for _, tr := range results {
		if tr.CallID == id {
			return true
		}
	}
	return false
}
