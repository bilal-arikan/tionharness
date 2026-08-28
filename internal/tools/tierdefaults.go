package tools

// TierDefaults is the declarative default-visibility layer of the tool
// visibility chain:
//
//	registration default (full) < bundle default < tool default
//	    < workspace ToolVisibility < agent ToolOverrides
//
// Bundle carries a per-BUNDLE base tier (see bundles.go), Tool a per-TOOL
// exception that wins over it. An empty tier string means "no opinion" — the
// name keeps whatever the registration default gave it (full). Neither layer
// can override a workspace or agent choice: both are applied at build time,
// before the override chain runs.
type TierDefaults struct {
	// Bundle maps a bundle key ("mcp:*", "group:automation") to a tier.
	Bundle map[string]string
	// Tool maps a tool NAME to a tier. Wins over Bundle.
	Tool map[string]string
}

// defaultBundleTiers is the per-bundle layer. It ships with exactly ONE row: the
// MCP wildcard, which reproduces AttachMCP's blanket name-only stamp — external
// servers can expose hundreds of tools, so their schemas load on demand and
// their (often multi-paragraph) descriptions are suppressed in the catalog.
//
// The row is kept explicit rather than hardcoded into TierFor so a later layer
// can override a SINGLE server ("mcp:playwright" -> summary) without touching
// the rest. Built-in "group:*" bundles deliberately ship with NO row: any value
// would change today's behavior, since the functional categories are not
// tier-homogeneous (group:files holds eager Read and name-only apply_patch
// alike). The mechanism exists so a workspace can later demote a whole group in
// one row.
var defaultBundleTiers = map[string]string{
	MCPBundleWildcard: VisibilityNameOnly,
}

// defaultToolTiers is the per-tool layer: a curated set of always-built tools
// that are self-descriptive AND used in only a minority of turns. They are
// listed in the load-on-demand catalog by NAME ALONE (no schema, no summary) —
// the Claude Code "deferred tool" style: the model recognises them by name and
// pulls the schema via tool_search / activate_tools when it actually needs one.
// This strips their schemas (and, for the formerly-eager ones, their per-turn
// cost entirely) from EVERY turn's cached prefix, while keeping them fully
// reachable. On the claude-cli path the Interaction MCP bridge still advertises
// the bridgeable ones with full schemas, so capability is unchanged there.
//
// A row for a name not built for the current agent is a harmless no-op, so
// gated tools (vault/config/session-context off) need no extra guarding — and
// the stamp is applied by NAME unconditionally, because VisibilityOf also
// classifies BRIDGED names that are not in this agent's builtins (see
// ApplyToolDefaults).
//
// Deliberately kept EAGER (behavioral nudges, context-bound, or high-frequency):
// todo_write, ask_user, request_confirmation, schedule_wake,
// create_artifact/update_artifact, use_skill/skill_search, run_subagent,
// Read/Write/Edit/list_dir/Glob/Grep, shell. The self-management suite is
// stamped hidden at build time (it is dependency-gated and therefore derived,
// not a static list) — more aggressive than name-only.
var defaultToolTiers = map[string]string{
	// Session lifecycle & navigation — names say it all; rarely the turn's point.
	"update_session": VisibilityNameOnly,
	"notify":         VisibilityNameOnly,
	"focus_view":     VisibilityNameOnly,
	// Cross-session & self-diagnostics — occasional, discoverable by name.
	"list_sessions":           VisibilityNameOnly,
	"conversation_search":     VisibilityNameOnly,
	"read_session_debug":      VisibilityNameOnly,
	"get_session_info":        VisibilityNameOnly,
	"update_user_preferences": VisibilityNameOnly,
	// Artifact revise + meta — create_artifact stays eager (behavioral); revise
	// and the deactivate meta-tool are reached on demand.
	"update_artifact":  VisibilityNameOnly,
	"deactivate_tools": VisibilityNameOnly,
	// archive_sessions — bulk housekeeping over OTHER sessions; a handful of calls
	// across the whole journal history, and the largest schema after get_view.
	"archive_sessions": VisibilityNameOnly,
	// expand — structural drill-down; get_view's EAGER description names it
	// explicitly ("the same ones `expand` hands you refs for"), so the model still
	// discovers it and pulls the schema when it actually fans out over the tree.
	"expand": VisibilityNameOnly,
	// apply_patch — the batch (multi-hunk/multi-file) sibling of Edit. Edit stays
	// eager, so single edits are unaffected; the batch path is pulled on demand.
	"apply_patch": VisibilityNameOnly,
	// Self-healing lessons — read/prune the auto-collected failure lessons. The
	// newest few already ride the context, so these are for deliberate inspection.
	"read_lessons":  VisibilityNameOnly,
	"delete_lesson": VisibilityNameOnly,
	// Insight (retrospective scanning) — self-descriptive names, used in a small
	// minority of turns; the model pulls a schema when it actually scans/triages.
	"insight_scan":          VisibilityNameOnly,
	"insight_list_findings": VisibilityNameOnly,
	"insight_apply_finding": VisibilityNameOnly,
	// Validation tools — read-only, used only around authoring/diagram emission.
	"skill_validate":   VisibilityNameOnly,
	"config_validate":  VisibilityNameOnly,
	"mermaid_validate": VisibilityNameOnly,
	// render_template — occasional (only when a branded-HTML skill is in play);
	// name says it, model pulls the schema on demand.
	"render_template": VisibilityNameOnly,
	// Background-shell management — reached only after a run_in_background launch.
	"shell_manage": VisibilityNameOnly,
	// Promoted out of the hidden self-management group: common enough to advertise
	// by name (handoff at context limit, DM a peer agent) rather than fold into the
	// self-management skill pointer. This table carries ONE authoritative entry per
	// name, so the promotion no longer depends on a "last mark wins" ordering — but
	// their MarkSelfManaged stamp is a separate, stable membership mark and MUST
	// still be applied, or the claude-cli bridge drops them entirely.
	"handoff_session": VisibilityNameOnly,
	"send_message":    VisibilityNameOnly,

	// Admin-rare tools fold into the HIDDEN tier (not enumerated per turn —
	// surfaced via the tionharness-self-management skill / tool_search). These are
	// confined config edits and secret reads: used in a tiny fraction of turns, and
	// their WRITE siblings (secret_set/secret_delete via the self-manage suite) are
	// already hidden — so hiding the reads keeps the secret/config family
	// consistent instead of split across the name-only and hidden tiers.
	//   - read/write/list_config : the agent editing its OWN prompts/instructions
	//   - secret                 : vault list/get/set/delete, only on credential tasks
	"read_config":  VisibilityHidden,
	"write_config": VisibilityHidden,
	"list_config":  VisibilityHidden,
	"secret":       VisibilityHidden,
}

// DefaultTiers returns the shipped default tiers as a fresh copy, so a caller
// may add or drop rows (e.g. a future workspace layer) without mutating the
// package-level tables.
func DefaultTiers() TierDefaults {
	d := TierDefaults{
		Bundle: make(map[string]string, len(defaultBundleTiers)),
		Tool:   make(map[string]string, len(defaultToolTiers)),
	}
	for k, v := range defaultBundleTiers {
		d.Bundle[k] = v
	}
	for k, v := range defaultToolTiers {
		d.Tool[k] = v
	}
	return d
}

// TierFor resolves the default tier for one tool name: its Tool entry, else the
// Bundle entry for its bundle, else the kind's wildcard row, else "" (no
// opinion — inherit the registration default).
func (d TierDefaults) TierFor(name string) string {
	if t := d.Tool[name]; t != "" {
		return t
	}
	return d.bundleTier(BundleOf(name))
}

// bundleTier resolves a bundle key against the Bundle layer, falling back to the
// kind's wildcard row ("mcp:*") when the bundle has none of its own.
func (d TierDefaults) bundleTier(key string) string {
	if t := d.Bundle[key]; t != "" {
		return t
	}
	kind, _, ok := SplitBundleKey(key)
	if !ok {
		return ""
	}
	switch kind {
	case "mcp":
		return d.Bundle[MCPBundleWildcard]
	case "group":
		return d.Bundle[GroupPrefix+bundleWildcard]
	}
	return ""
}

// ApplyToolDefaults stamps the per-TOOL default tiers by NAME, unconditionally —
// a name this agent never built is recorded exactly as the hand-written
// MarkNameOnly/MarkHidden calls recorded it. That is load-bearing, not sloppy:
// VisibilityOf is handed to the API layer as Runtime.ToolVisibilityFunc and is
// asked to classify BRIDGED tool names on the claude-cli wire, which need not be
// among this registry's builtins. Restricting the stamp to registered names
// would silently reclassify those.
//
// Rows with an empty tier are skipped (no opinion), as is an unknown tier value
// (SetVisibility rejects it).
func (r *Registry) ApplyToolDefaults(d TierDefaults) {
	for name, tier := range d.Tool {
		if tier == "" {
			continue
		}
		r.SetVisibility(name, tier)
	}
}

// ApplyBundleDefaults stamps the per-BUNDLE default tier over every name the
// registry currently KNOWS (built-ins + attached MCP entries) that has no
// Tool-level entry of its own. Unlike ApplyToolDefaults this needs an enumerable
// set, which is fine: the bundle layer is new, so nothing outside the registry
// ever depended on it.
//
// Call it AFTER AttachMCP, or the MCP entries do not exist yet.
func (r *Registry) ApplyBundleDefaults(d TierDefaults) {
	if len(d.Bundle) == 0 {
		return
	}
	apply := func(name string) {
		if d.Tool[name] != "" {
			return // the per-tool layer wins
		}
		if tier := d.bundleTier(BundleOf(name)); tier != "" {
			r.SetVisibility(name, tier)
		}
	}
	for name := range r.builtins {
		apply(name)
	}
	for _, e := range r.mcpEntries {
		apply(e.NamespacedName)
	}
}
