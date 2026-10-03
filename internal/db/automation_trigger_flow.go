package db

import "fmt"

// Flow trigger kind (_Docs/93): an automation that fires when a flow run — one
// turn of an agent's evolving flow — finishes. It is the event-side complement
// of the flow's trigger NODE (which fires an automation from inside a run):
// "when agent X's flow fails / is graded poorly / finishes, do Y". The rule
// may narrow on the flow's agent, the run's outcome and the decision model's
// grade. Spawn tags are ignored (a flow rule never tag-loops); a run whose
// session this very rule spawned does not re-fire it (engine side).
const TriggerFlow = "flow"

// ValidFlowStatus reports whether st is empty (any outcome) or a terminal run
// status a flow rule may narrow on.
func ValidFlowStatus(st string) bool {
	switch st {
	case "", FlowSuccess, FlowFailure:
		return true
	}
	return false
}

func init() {
	RegisterTrigger(TriggerSpec{
		Kind: TriggerFlow, Label: "Akış koşusu",
		Validate: func(a Automation) error {
			if !ValidFlowStatus(a.FlowStatus) {
				return fmt.Errorf("%w: invalid flowStatus %q (success|failure or empty for any)", ErrAutomationShape, a.FlowStatus)
			}
			if a.FlowMaxGrade < 0 || a.FlowMaxGrade > 5 {
				return fmt.Errorf("%w: flowMaxGrade %d must be 0 (ignore) or 1..5", ErrAutomationShape, a.FlowMaxGrade)
			}
			return nil
		},
	})
}
