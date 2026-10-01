package agent

import "github.com/bilal-arikan/tionharness/internal/decider"

const (
	authSessionSetup     = "session-setup"
	authModelRouter      = "model-router"
	authClarification    = "clarification"
	authCompactRetention = "compact-retention"
	authContextReminder  = "context-reminder"
	authWorkerReview     = "worker-review"
)

func init() {
	for i, a := range []decider.Authority{
		{ID: authSessionSetup, Group: decider.GroupSession, Pattern: decider.PatternSelect, Label: "Skills and tools", Description: "Selects permitted skills and tools together at session start, loads skill instructions and activates tool schemas in one batch."},
		{ID: authModelRouter, Group: decider.GroupSession, Pattern: decider.PatternPick, Label: "Execution model routing", Description: "Selects a configured, available execution model on the first turn. Keeps the session route stable and respects a pinned model."},
		{ID: authClarification, Group: decider.GroupSession, Pattern: decider.PatternGate, Label: "Clarification check", Description: "Checks optional clarification questions against the recent conversation. Required input and action approvals always reach the user."},
		{ID: authCompactRetention, Group: decider.GroupContext, Pattern: decider.PatternTriage, Label: "Compact context protection", Description: "Labels older context as keep, summarize or drop before a managed fold. Pinned fragments remain available; the original transcript is preserved."},
		{ID: authContextReminder, Group: decider.GroupContext, Pattern: decider.PatternSelect, Label: "Context reminders", Description: "After the configured number of managed or CLI compacts, selects saved context fragments to bring back into the next turn."},
		{ID: authWorkerReview, Group: decider.GroupCollaboration, Pattern: decider.PatternSelect, Label: "Worker result review", Description: "Selects verification, alternatives and synthesis perspectives for the coordinator when a worker result arrives. Delivery never depends on the judge."},
	} {
		a.Modes = []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn}
		a.DefaultMode = decider.ModeOff
		a.DefaultThreshold = 0.8
		a.ThresholdHint = "Minimum confidence before a recommendation changes the current workflow. Lower confidence keeps the existing behavior."
		a.Order = i
		decider.RegisterAuthority(a)
	}
}
