package providers

import (
	"strings"
	"time"
)

// Per-request wall-clock budgets are model-class aware: reasoning-heavy classes
// can legitimately keep a SINGLE request running for minutes, so killing them at
// the default 120s would abort mid-generation and re-bill the work via the
// transport retry. This file is the one place that decides the budget; every
// HTTP provider client (native Anthropic and OpenAI-compatible) consults it so
// the deadline policy is defined once. The two budget consts live in anthropic.go
// (requestTimeoutSecs / adaptiveRequestTimeoutSecs / clientTimeoutSecs).

// LongRequestModel reports whether a model needs the long per-request budget.
// It is deliberately SEPARATE from UsesAdaptiveThinking (an Anthropic wire-format
// concern): a reasoning model on an OpenAI-compatible endpoint — DeepSeek V4 Pro,
// served via the shared OpenAICompat client — needs the long budget without
// speaking the adaptive-thinking protocol. Every adaptive-thinking model also
// qualifies (a single Fable/Opus request can think for minutes).
func LongRequestModel(model string) bool {
	if UsesAdaptiveThinking(model) {
		return true
	}
	m := strings.ToLower(model)
	for _, s := range []string{"deepseek-v4-pro", "deepseek-reasoner", "-reasoner"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// RequestTimeoutSecsFor returns the model-class default per-request budget in
// seconds: the long budget for reasoning/adaptive models, the historical 120s
// otherwise.
func RequestTimeoutSecsFor(model string) int {
	if LongRequestModel(model) {
		return adaptiveRequestTimeoutSecs
	}
	return requestTimeoutSecs
}

// effectiveRequestTimeoutSecs resolves the per-request budget: a positive
// override (a kind Manifest's RequestTimeoutSecs, applied by Registry.Get, or a
// direct WithRequestTimeout call) always wins; otherwise the model-class default
// applies.
func effectiveRequestTimeoutSecs(overrideSecs int, model string) int {
	if overrideSecs > 0 {
		return overrideSecs
	}
	return RequestTimeoutSecsFor(model)
}

// clientSafetyTimeout returns the http.Client transport timeout to pair with a
// per-request budget: kept above the budget so the ctx deadline (model-class
// aware) stays the binding limit and the transport is only a last-resort net.
func clientSafetyTimeout(reqSecs int) time.Duration {
	return time.Duration(reqSecs+30) * time.Second
}

// requestTimeoutConfigurable is implemented by HTTP provider clients whose
// per-request budget can be overridden declaratively (a kind's Manifest, later a
// settings field). Registry.Get applies the override after Build, so no per-kind
// build function has to thread it. secs <= 0 means "keep the model-class default".
type requestTimeoutConfigurable interface {
	setRequestTimeout(secs int)
}
