package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
// This is the LENIENT reader, for DISPLAY surfaces only (the agent tools screen,
// the tier diff): malformed JSON is dropped rather than fatal, so a corrupt
// override document does not make an agent unrenderable. Every PERMISSION path
// (blockFunc, mcpServerGate) must call ParseToolOverridesErr instead — silently
// degrading an unreadable denylist into "nothing blocked" is a fail-OPEN.
func ParseToolOverrides(agent db.Agent) map[string]string {
	out, _ := ParseToolOverridesErr(agent)
	return out
}

// ParseToolOverridesErr is ParseToolOverrides plus the parse error, so a caller
// that enforces permissions can refuse to run on a document it could not read.
// The map it returns alongside an error holds only the fields that DID parse; a
// caller must treat a non-nil error as "this agent's overrides are unknown" and
// fail closed, never as "no overrides".
//
// A blank field is not an error — it means the agent set nothing.
func ParseToolOverridesErr(agent db.Agent) (map[string]string, error) {
	out := map[string]string{}
	var errs []error
	if raw := strings.TrimSpace(agent.ToolOverrides); raw != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
			errs = append(errs, fmt.Errorf("tool_overrides: %w", err))
		} else {
			for name, tier := range overrides {
				if ValidAgentTier(tier) {
					out[name] = tier
				} else {
					errs = append(errs, fmt.Errorf("tool_overrides: tool %q has unknown tier %q", name, tier))
				}
			}
		}
	}
	if raw := strings.TrimSpace(agent.BlockedTools); raw != "" {
		var legacy []string
		if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
			errs = append(errs, fmt.Errorf("blocked_tools: %w", err))
		} else {
			for _, name := range legacy {
				if _, explicit := out[name]; !explicit {
					out[name] = TierBlocked
				}
			}
		}
	}
	return out, errors.Join(errs...)
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

// applyVisibilityOverrides pins each override onto the registry. Three key kinds
// are supported: an exact tool name, a "prefix*" PATTERN, and a
// "group:<category>" GROUP key. SetVisibility takes one exact name, so both
// pattern and group keys are expanded across the catalog and applied to every
// matching tool. Exact keys are applied directly (an unknown name is a harmless
// no-op, matching the workspace override behaviour) so a tool that is not built
// for this agent — e.g. a gated shell tool — can still carry a stored override
// for when it does appear.
//
// Specificity is enforced in TWO EXPLICIT PASSES, not by sort order: all broad
// keys (patterns + groups) are applied first, then the exact names, so an exact
// name ALWAYS wins over a group or pattern that also covers it. Within the broad
// pass the keys are sorted so that two overlapping patterns (e.g. "mcp*" and
// "mcp__linear*") resolve deterministically — sorting puts the more specific
// (longer) pattern last, so it wins. Groups and patterns cannot overlap in
// practice: a group only matches non-namespaced built-ins, but if a user writes
// both, sort order still makes the outcome stable rather than random.
//
// names is the registry's full catalog, needed only to expand patterns/groups.
func applyVisibilityOverrides(reg *tools.Registry, overrides map[string]string, names []string) {
	broad := make([]string, 0, len(overrides))
	exact := make([]string, 0, len(overrides))
	for k := range overrides {
		if tools.IsGroupKey(k) || strings.HasSuffix(k, "*") {
			broad = append(broad, k)
		} else {
			exact = append(exact, k)
		}
	}
	sort.Strings(broad)
	sort.Strings(exact)

	for _, key := range broad {
		tier := overrides[key]
		if tools.IsGroupKey(key) {
			for _, name := range names {
				if tools.MatchesGroup(name, key) {
					reg.SetVisibility(name, tier)
				}
			}
			continue
		}
		prefix := strings.TrimSuffix(key, "*")
		for _, name := range names {
			if strings.HasPrefix(name, prefix) {
				reg.SetVisibility(name, tier)
			}
		}
	}
	for _, key := range exact {
		reg.SetVisibility(key, overrides[key])
	}
}
