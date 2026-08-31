package conversation

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// EstimatePersistedStepTokens reports the tokens a CLI provider still holds in
// its own thread for the given messages: the persisted Steps trace that
// EstimateTokens deliberately excludes because TionHarness never resends it.
//
// This is the single numerator for that term. A CLI resume ships only the new
// delta, so the provider-side thread keeps every intermediate tool call and
// result of the earlier turns — invisible to EstimateTokens, yet very much
// inside the model's window. Callers that budget or meter a WARM CLI thread add
// this on top of EstimateTokens; callers that talk to a native provider (or a
// cold CLI turn, where nothing is retained yet) must not.
//
// A malformed persisted trace is returned as an error rather than counted as
// zero: silently dropping it is exactly how the footprint went unnoticed.
func EstimatePersistedStepTokens(msgs []db.Message) (int, error) {
	total := 0
	for _, m := range msgs {
		if m.Role != providers.RoleAssistant {
			continue
		}
		tokens, _, err := EstimatePersistedSteps(m.Steps)
		if err != nil {
			return 0, err
		}
		total += tokens
	}
	return total, nil
}
