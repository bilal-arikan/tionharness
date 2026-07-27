package agent

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TierBlocked is the per-agent override tier that removes a tool from the
// agent's catalog entirely. It is NOT a visibility tier: the registry only ever
// sees the four tools.Visibility* values, and "blocked" is resolved one layer
// up, in the tool filter. Merging it into the same map is what unified the
// former standalone BlockedTools denylist with the visibility model.
const TierBlocked = db.TierBlocked

// ValidAgentTier reports whether tier is one of the five per-agent override
// values: the four context-visibility tiers plus "blocked".
func ValidAgentTier(tier string) bool {
	switch tier {
	case tools.VisibilityFull, tools.VisibilitySummary, tools.VisibilityNameOnly,
		tools.VisibilityHidden, TierBlocked:
		return true
	default:
		return false
	}
}

// ParseToolOverrides returns an agent's effective tool override map (tool name
// or "prefix*" pattern → tier), folding the LEGACY BlockedTools denylist in as
// "blocked" entries.
//
// The two sources are merged rather than one shadowing the other: BlockedTools
// is a derived mirror written by UpdateAgentTools, so for any agent saved by the
// current build the two agree. For an agent written by an older build (override
// map empty, denylist populated) the merge is the migration. An explicit entry
// in ToolOverrides always wins — so an agent whose override map deliberately
// UNBLOCKS a tool the stale mirror still lists is honoured.
//
// Malformed JSON in either field is ignored rather than fatal: a corrupt
// override file must not make an agent unrunnable, it must fall back to "no
// overrides" (its tools then follow the workspace-effective tiers).
func ParseToolOverrides(agent db.Agent) map[string]string {
	out := map[string]string{}
	var overrides map[string]string
	if err := json.Unmarshal([]byte(agent.ToolOverrides), &overrides); err == nil {
		for name, tier := range overrides {
			if ValidAgentTier(tier) {
				out[name] = tier
			}
		}
	}
	var legacy []string
	if err := json.Unmarshal([]byte(agent.BlockedTools), &legacy); err == nil {
		for _, name := range legacy {
			if _, explicit := out[name]; !explicit {
				out[name] = TierBlocked
			}
		}
	}
	return out
}

// blockedPatterns returns the sorted name/pattern keys carrying the "blocked"
// tier — the denylist the tool filter enforces. Sorted so the derived predicate
// (and any log of it) is deterministic.
func blockedPatterns(overrides map[string]string) []string {
	var out []string
	for name, tier := range overrides {
		if tier == TierBlocked {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// visibilityOverrides returns the override entries that ARE visibility tiers
// (everything except "blocked"), keyed by name/pattern.
func visibilityOverrides(overrides map[string]string) map[string]string {
	out := make(map[string]string, len(overrides))
	for name, tier := range overrides {
		if tier != TierBlocked {
			out[name] = tier
		}
	}
	return out
}

// applyVisibilityOverrides pins each override onto the registry. A key ending in
// "*" is a PREFIX PATTERN: SetVisibility takes one exact name, so the pattern is
// expanded across the catalog and applied to every matching tool. Exact keys are
// applied directly (an unknown name is a harmless no-op, matching the workspace
// override behaviour) so a tool that is not built for this agent — e.g. a gated
// shell tool — can still carry a stored override for when it does appear.
//
// names is the registry's full catalog, needed only to expand patterns.
func applyVisibilityOverrides(reg *tools.Registry, overrides map[string]string, names []string) {
	// Deterministic order: two patterns can match the same tool (e.g. "mcp*" and
	// "mcp__linear*"), and map iteration would make the winner random. Sorting
	// puts the more specific (longer) pattern last, so it wins.
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		tier := overrides[key]
		if !strings.HasSuffix(key, "*") {
			reg.SetVisibility(key, tier)
			continue
		}
		prefix := strings.TrimSuffix(key, "*")
		for _, name := range names {
			if strings.HasPrefix(name, prefix) {
				reg.SetVisibility(name, tier)
			}
		}
	}
}
