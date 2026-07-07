package conversation

import (
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/providers"
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
	rebuilt := make([]providers.Message, 0, len(out)+1)
	structural := false
	for i := 0; i < len(out); i++ {
		m := out[i]
		if m.Role == providers.RoleUser && len(m.ToolResults) > 0 {
			// Valid ids = calls of the immediately preceding assistant message
			// (the provider contract); anything else is an orphan.
			valid := map[string]bool{}
			if len(rebuilt) > 0 {
				prev := rebuilt[len(rebuilt)-1]
				if prev.Role == providers.RoleAssistant {
					for _, tc := range prev.ToolCalls {
						valid[tc.ID] = true
					}
				}
			}
			kept := m.ToolResults[:0:0]
			for _, tr := range m.ToolResults {
				if !valid[tr.CallID] {
					notes = append(notes, RepairNote{Rule: "orphan_tool_result", Detail: "call " + tr.CallID})
					structural = true
					continue
				}
				kept = append(kept, tr)
			}
			if len(kept) == 0 && m.Text == "" {
				structural = true
				continue // nothing left — drop the message entirely
			}
			m.ToolResults = kept
			rebuilt = append(rebuilt, m)
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
					structural = true
					rebuilt = append(rebuilt, merged)
					i++ // consumed the next message too
					// The merged batch is still unanswered; the next loop pass (below)
					// cannot revisit it, so synthesize its results here if the message
					// after the pair is not a result message.
					if i+1 >= len(out) || out[i+1].Role != providers.RoleUser || len(out[i+1].ToolResults) == 0 {
						rebuilt = append(rebuilt, syntheticResults(merged.ToolCalls))
						notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized (post-merge)", len(merged.ToolCalls))})
					}
					continue
				}
				// No mergeable neighbour: answer the batch with synthetic
				// cancelled results so the pair invariant holds.
				notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized", len(m.ToolCalls))})
				structural = true
				rebuilt = append(rebuilt, m, syntheticResults(m.ToolCalls))
				continue
			}
			// Answered — but only for the ids the next message actually carries;
			// synthesize the gap so partial results (mid-batch crash) still pair up.
			have := map[string]bool{}
			for _, tr := range out[i+1].ToolResults {
				have[tr.CallID] = true
			}
			missing := 0
			for _, tc := range m.ToolCalls {
				if !have[tc.ID] {
					missing++
				}
			}
			if missing > 0 {
				// Rebuild the answer: keep only results matching this batch's ids
				// (anything else is an orphan that would 400 anyway), then fill
				// the gaps with synthetic cancelled results.
				valid := map[string]bool{}
				for _, tc := range m.ToolCalls {
					valid[tc.ID] = true
				}
				next := out[i+1]
				results := make([]providers.ToolResult, 0, len(m.ToolCalls))
				for _, tr := range next.ToolResults {
					if valid[tr.CallID] {
						results = append(results, tr)
					} else {
						notes = append(notes, RepairNote{Rule: "orphan_tool_result", Detail: "call " + tr.CallID})
					}
				}
				for _, tc := range m.ToolCalls {
					if !have[tc.ID] {
						results = append(results, providers.ToolResult{CallID: tc.ID, Content: repairCancelledMsg, IsError: true})
					}
				}
				next.ToolResults = results
				notes = append(notes, RepairNote{Rule: "missing_tool_result", Detail: fmt.Sprintf("%d synthesized (partial batch)", missing)})
				structural = true
				rebuilt = append(rebuilt, m, next)
				i++
				continue
			}
		}
		rebuilt = append(rebuilt, m)
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
