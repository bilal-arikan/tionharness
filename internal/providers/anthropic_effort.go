package providers

// legacyThinkingBudgetCap is the largest budget_tokens the enabled+budget shape
// is sent with: the xhigh/max tiers exist only as effort levels, so their
// budgets are clamped rather than sent as-is (see thinkingFor).
const legacyThinkingBudgetCap = 16384

// coarseEffortThinking builds the Messages-API reasoning controls for the
// coarse-effort class (DeepSeek V4.x, GLM-5.3 — see UsesCoarseEffort). Depth
// rides output_config.effort because both endpoints ignore budget_tokens; the
// enabled+budget shape is still sent next to it — it is the form both endpoints
// already accepted on the legacy path — and max_tokens is bumped above the
// budget exactly as there.
//
// Off is explicit because both vendors think by default: {type:"disabled"} for
// DeepSeek, and for the always-reasoning GLM-5.3 family (where "disabled" fails
// the request) no thinking field at all plus the lowest effort. The native tool
// loop always runs with a zero budget, so this is also what every tool turn
// sends: DeepSeek tool turns carry no reasoning to echo back, and GLM-5.3 tool
// turns reason at "low" instead of its slow "max" default.
func coarseEffortThinking(model string, budget, maxTokens int) (*thinkingParam, *outputConfig, int) {
	forced := ForcedThinking(model)
	if budget <= 0 {
		if forced {
			return nil, &outputConfig{Effort: CoarseEffortForBudget(0, true)}, maxTokens
		}
		return &thinkingParam{Type: "disabled"}, nil, maxTokens
	}
	// The effort comes from the unclamped budget: xhigh/max must reach "max".
	cfg := &outputConfig{Effort: CoarseEffortForBudget(budget, forced)}
	if budget > legacyThinkingBudgetCap {
		budget = legacyThinkingBudgetCap
	}
	if maxTokens <= budget {
		maxTokens = budget + defaultMaxTokens
	}
	return &thinkingParam{Type: "enabled", BudgetTokens: budget}, cfg, maxTokens
}
