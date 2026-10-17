package providers

import "strings"

// BareModelForKind strips opencode's "<vendor>/<model>" prefix for the
// opencode-cli kind so the family-keyed tables (context window, output cap,
// thinking tiers, pricing) see the model id the way every other kind spells it.
// Nested vendor paths ("openrouter/anthropic/claude-…") keep only the last
// segment. Every other kind returns model unchanged.
func BareModelForKind(providerKind, model string) string {
	if providerKind != "opencode-cli" {
		return model
	}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		return model[i+1:]
	}
	return model
}
