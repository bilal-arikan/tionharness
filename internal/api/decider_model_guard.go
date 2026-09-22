package api

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// rejectDecisionModel refuses a decision-only model (TypeSafe Jev and its kin)
// as an agent's chat model. The model picker accepts free-typed ids, and the
// chat endpoint rejects these models on every turn ("… is a decisions model and
// cannot be used with the chat/completions endpoint"), so the agent would be
// saved and then fail forever. Decision models are configured on the decider's
// own settings page instead.
func rejectDecisionModel(model string) error {
	if decider.IsDecisionModel(model) {
		return fmt.Errorf("%q is a decision model (typed yes/no, choice and score answers) and cannot run an agent; set it under Settings → Decision model instead", model)
	}
	return nil
}
