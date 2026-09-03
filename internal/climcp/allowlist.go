package climcp

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// NativeToolAllowlist returns the claude-cli BUILT-IN tools a bridged turn may
// keep, for the `--tools` flag. It is the positive mirror of the suppression list
// WriteConfig assembles: the same conditions, inverted. The point is prompt size,
// not permission — `--disallowedTools` leaves the CLI's system prompt at its
// full ~36k tokens (the suppressed tools still shape it), while `--tools` with
// the five file tools + ToolSearch measures ~11k (2.1.259, _Docs/17). Every entry
// below is either always needed (file tools; ToolSearch, which reaches the
// deferred MCP tiers; the MCP resource readers) or kept ONLY while its bridged
// replacement is absent, exactly as the suppression logic keeps the native
// fallback alive in that case. Deterministic order: it feeds the persistent-
// session fingerprint.
func NativeToolAllowlist(ag db.Agent, inter tools.InteractionEndpoint, mode string, shellEnabled bool) []string {
	out := []string{"Read", "Edit", "Write", "Glob", "Grep", "NotebookEdit", "ToolSearch",
		"ListMcpResourcesTool", "ReadMcpResourceTool"}
	if ag.NativeWebSearchEnabled() {
		out = append(out, "WebSearch", "WebFetch")
	}
	advertised := advertisedSet(inter)
	// Native shell family survives only while TionHarness's own shell is not bridged.
	if !shellEnabled {
		out = append(out, shellFamily...)
	}
	// WS17 invariant (see WriteConfig): a native whose bridge is not advertised
	// this turn stays on the menu so the model is never left without the family.
	if !advertised["todo_write"] {
		out = append(out, todoFamily...)
	}
	if !advertised["use_skill"] {
		out = append(out, "Skill")
	}
	// Plan mode only completes when its exit approval can be answered — wired in
	// "ask" and "read-only" (PromptToolForMode); elsewhere it would hang headless.
	if mode == "ask" || mode == "read-only" {
		out = append(out, planFamily...)
	}
	return out
}
