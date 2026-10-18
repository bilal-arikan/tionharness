package api

import (
	"github.com/bilal-arikan/tionharness/internal/agent"
)

func cliContinuationStep(plan cliResumePlan) *agent.TurnStep {
	if plan.reason == "" {
		return nil
	}
	status := "disabled"
	if plan.active {
		status = "cold"
		if !plan.coldStart {
			status = "warm"
		}
	}
	return &agent.TurnStep{Kind: agent.StepText, Operation: "cli_resume", Status: status, Reason: plan.reason,
		Text: "CLI context continuity", Provider: "codex-cli"}
}
