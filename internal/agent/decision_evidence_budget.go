package agent

import (
	"encoding/json"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Only evidence text may shrink. Question keys, candidates, roles and fixed
// classifier instructions remain intact; the Hub still validates the result.
func (r *Runtime) fitDecisionEvidence(authority string, req decider.Request) decider.Request {
	state, ok := req.State.(map[string]any)
	if !ok {
		return req
	}
	budget := r.deciderHub().StateBudget(authority, req.Questions)
	for round := 0; round < 16; round++ {
		raw, err := json.Marshal(state)
		if err != nil || len(raw) <= budget {
			return req
		}
		changed := false
		shrink := func(text string) string {
			if text != "" {
				changed = true
			}
			return policyText(text, len(text)/2)
		}
		if e, ok := state["sessionContext"].(decisionEvidenceState); ok {
			e.InitialRequest, e.Summary = shrink(e.InitialRequest), shrink(e.Summary)
			for i := range e.RecentConversation {
				e.RecentConversation[i].Text = shrink(e.RecentConversation[i].Text)
			}
			for _, row := range e.ProtectedContext {
				row["text"] = shrink(row["text"])
			}
			state["sessionContext"] = e
		}
		for _, key := range []string{"task", "summary", "workerResult"} {
			if text, ok := state[key].(string); ok {
				state[key] = shrink(text)
			}
		}
		if fragments, ok := state["fragments"].([]map[string]any); ok {
			for _, row := range fragments {
				if text, ok := row["text"].(string); ok {
					row["text"] = shrink(text)
				}
			}
		}
		if fragments, ok := state["retainedFragments"].([]map[string]string); ok {
			for _, row := range fragments {
				row["text"] = shrink(row["text"])
			}
		}
		state["evidenceTruncated"] = true
		if !changed {
			break
		}
	}
	return req
}
