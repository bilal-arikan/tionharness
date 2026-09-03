package conversation

import "github.com/bilal-arikan/tionharness/internal/providers"

// EstimateToolDefTokens approximates the token footprint of the tool schemas
// shipped with a request. EstimateProviderTokens deliberately walks messages
// only, which is right for a fold (a fold cannot shrink the tools block) but
// wrong for deciding whether a reduced history now FITS: the model receives
// messages plus these schemas, and in native-search mode the shipped set is the
// whole deferred catalog — tens of thousands of tokens that the message-only
// figure does not see.
//
// This is the in-flight analogue of the turn-level contextOverheadFrom term.
// Name, description and the raw JSON schema are all counted; they are all sent.
func EstimateToolDefTokens(defs []providers.ToolDef) int {
	total := 0
	for _, d := range defs {
		total += estimateText(d.Name) + estimateText(d.Description) + estimateText(string(d.InputSchema)) + msgOverhead
	}
	return total
}

// EstimateInFlightTokens is the footprint one in-flight request actually
// carries: the message history plus the tool schemas riding with it. It is the
// figure to weigh against a model's context window mid-loop.
func EstimateInFlightTokens(msgs []providers.Message, defs []providers.ToolDef) int {
	return EstimateProviderTokens(msgs) + EstimateToolDefTokens(defs)
}
