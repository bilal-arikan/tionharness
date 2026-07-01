package tools

import (
	"sort"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// bridge_filter.go — POC: optional hidden-tier exclusion for the claude-cli
// Interaction MCP bridge.
//
// Background. On the native tool-loop the 4 visibility tiers (full / summary /
// name-only / hidden) each render differently, and a hidden tool stays
// activatable in-turn via activate_tools. Over the CLI bridge that granularity
// collapses: a tool MUST ship its full schema to be callable over MCP, and the
// CLI's --allowedTools set is fixed at process start, so there is no in-turn
// "activate a not-yet-advertised tool" step. BridgeableDefs therefore bridges
// ALL lazy built-ins (summary + name-only + hidden alike) with full schemas and
// leans on the CLI's own ENABLE_TOOL_SEARCH=auto to defer whatever overflows.
//
// This POC lets us ask: what if the hidden tier (the self-management suite,
// dozens of tools) is NOT bridged to the CLI at all? Then those schemas never
// travel to the CLI process for that turn. The tools are not callable in-turn;
// the CLI-side analogue of "activate" becomes a NEXT-turn re-allowlist (SwarmGo
// re-advertises them once the model/user asks for them). skipHidden=false
// reproduces BridgeableDefs exactly. The runtime gate that drives this defaults
// to ON as of 2026-07-01 (Tunables.cliBridgeSkipHidden).

// BridgeableDefsFiltered returns the full schemas of lazy built-in tools for the
// CLI Interaction MCP bridge, mirroring BridgeableDefs, with one extra knob:
// when skipHidden is true, tools marked hidden (r.hidden — the self-management
// default tier) are omitted entirely. MCP tools are excluded (the CLI reaches
// those through their own server entries); bridgeExcluded tools are skipped
// (CLI-native or native-loop-context-bound). allow filters by name (nil = allow
// all), matching the per-agent tool filter used on the native path.
func (r *Registry) BridgeableDefsFiltered(allow func(name string) bool, skipHidden bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if !r.lazy[name] || bridgeExcluded[name] {
			continue
		}
		if skipHidden && r.hidden[name] {
			continue
		}
		if allow != nil && !allow(name) {
			continue
		}
		out = append(out, foldExamples(t.Def()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HiddenBridgeableCount reports how many bridgeable (lazy, non-excluded) built-in
// tools are hidden-tier — i.e. how many BridgeableDefsFiltered(_, true) drops
// versus BridgeableDefsFiltered(_, false). It exists purely to measure the POC:
// the caller can log "skipped N hidden tools" to size the win. allow matches the
// filter used above.
func (r *Registry) HiddenBridgeableCount(allow func(name string) bool) int {
	n := 0
	for name := range r.builtins {
		if !r.lazy[name] || bridgeExcluded[name] || !r.hidden[name] {
			continue
		}
		if allow != nil && !allow(name) {
			continue
		}
		n++
	}
	return n
}
