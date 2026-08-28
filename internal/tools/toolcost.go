package tools

// Per-turn TOKEN COST of tool schemas.
//
// A tool is not free just because it is never called: whatever the model can see
// of it rides in every request. This file turns that into a number so the tools
// screen can answer "what does pulling this whole group up to `full` cost me per
// turn?" instead of showing a bare tool count.
//
// How a tier is priced (mirrors what request assembly actually ships):
//
//	full       name + description + the SERIALIZED InputSchema, with Examples
//	           folded in exactly as foldExamples does at request-assembly time
//	           (examples travel with the full schema, never with the catalog),
//	           plus a small fixed per-def framing overhead.
//	summary    name + description only — the load-on-demand catalog line.
//	name-only  name only — the catalog lists it without its summary.
//	hidden     0 — folded out of the catalog entirely; the agent pays nothing
//	           until tool_search discovers it.
//
// The unit is conversation.EstimateText's approximation, the same estimator the
// context preview uses — so these numbers are comparable with the rest of the UI
// and are ESTIMATES, not a billing figure. Treat them as relative: "this group
// costs ~4x that one" is the claim, not "this group costs exactly N tokens".

import (
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// defFramingTokens is the fixed per-tool cost of the JSON envelope a tool def is
// wrapped in (field keys, braces, separators) — small, but it is the difference
// between "20 tiny tools are free" and "20 tiny tools are not".
const defFramingTokens = 4

// TierTokens estimates what ONE tool def costs per turn at the given visibility
// tier. An unknown tier is priced as VisibilityHidden (zero): callers resolve the
// tier from the registry, so an unrecognised value means the tool is not being
// advertised at all.
func TierTokens(d providers.ToolDef, tier string) int {
	switch tier {
	case VisibilityFull:
		// foldExamples is idempotent — folding an already-folded def rewrites the
		// same "examples" key — so this is safe on defs that came from prepDef.
		folded := foldExamples(d)
		return defFramingTokens +
			conversation.EstimateText(folded.Name) +
			conversation.EstimateText(folded.Description) +
			conversation.EstimateText(string(folded.InputSchema))
	case VisibilitySummary:
		return defFramingTokens +
			conversation.EstimateText(d.Name) +
			conversation.EstimateText(d.Description)
	case VisibilityNameOnly:
		return defFramingTokens + conversation.EstimateText(d.Name)
	default:
		return 0
	}
}

// FullSchemaTokens estimates the per-turn cost of shipping every def in the slice
// at the `full` tier — the "what would this group cost me" figure. An empty slice
// costs nothing.
func FullSchemaTokens(defs []providers.ToolDef) int {
	total := 0
	for _, d := range defs {
		total += TierTokens(d, VisibilityFull)
	}
	return total
}

// CurrentTokens estimates what the defs cost per turn AS THEY STAND, pricing each
// one at the tier tierOf reports for it. tierOf must be non-nil: a missing
// resolver is a programming error, not a reason to silently report zero cost.
func CurrentTokens(defs []providers.ToolDef, tierOf func(name string) string) int {
	if tierOf == nil {
		panic("tools.CurrentTokens: tierOf resolver is nil")
	}
	total := 0
	for _, d := range defs {
		total += TierTokens(d, tierOf(d.Name))
	}
	return total
}
