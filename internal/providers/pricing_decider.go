package providers

// Decision-model prices (internal/decider) beyond the two Jev ids in the
// OpenRouter block of priceTable. Kept in their own file and merged at init,
// like the market pack prices, so the decision layer's billing does not ride on
// edits to the main table.
func init() {
	// OpenRouter's System One endpoint maps TypeSafe's bare ids onto the
	// typesafe/ namespace; the floating alias is billed like the snapshot.
	priceTable["openrouter"]["typesafe/jev-latest"] = Price{InputPerMTok: 0.042, OutputPerMTok: 0}
	// TypeSafe's own System One API (decider backend "systemone"): the same
	// input-only price as through OpenRouter.
	priceTable["typesafe"] = map[string]Price{
		"jev-latest": {InputPerMTok: 0.042, OutputPerMTok: 0},
		"jev-1.13":   {InputPerMTok: 0.042, OutputPerMTok: 0},
	}
}
