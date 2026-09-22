package agent

// searchIndexToolName is the bare name of the index-lifecycle tool the
// capability blocks point at.
const searchIndexToolName = "search_index"

// searchIndexToolFor returns the name the agent must actually CALL search_index
// by, for the given provider.
//
// On the CLI path TionHarness's own built-ins reach the model through the
// Interaction MCP server, so the callable name carries that namespace
// (mcp__tionharness_extended__search_index); on the native path the bare name
// is correct. This is the same per-provider naming codebaseMemoryGuidance
// already threads `provider` for, and for the same reason: a block that names a
// tool the model cannot call by that name sends it hunting, or back to the
// shell — which is precisely the behaviour these blocks forbid.
func searchIndexToolFor(provider string) string {
	if isCLIProviderKind(provider) {
		return extendedToolPrefix + searchIndexToolName
	}
	return searchIndexToolName
}

// searchIndexDirective is the shared sentence pair both capability blocks end
// with: where to send an agent whose index is missing or stale, and the
// prohibition that makes the tool necessary.
//
// It is one string rather than two copies because the two blocks are written by
// different files and drifted apart once already — the zvec-grep block said
// "never index yourself" while offering no alternative at all.
func searchIndexDirective(provider, indexer string) string {
	tool := searchIndexToolFor(provider)
	return "If the index is missing or stale, call `" + tool + "` (action `status`, then `refresh`, " +
		"or `rebuild` when the store is corrupt) and TionHarness performs it for you — the run is " +
		"asynchronous, so re-check with `status` rather than waiting. Never run `" + indexer +
		"` yourself through the shell, and never delete an index: dropping one is a user action."
}
