// Package tools provides the agent tool registry: a unified catalog of
// built-in (in-process) tools plus tools sourced from connected MCP servers.
// The registry produces provider.ToolDef schemas for a completion request and
// executes provider.ToolCall results, regardless of a tool's origin.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// maxToolOutputBytes bounds a tool's output before it is fed back to the model
// and persisted to the session JSONL. Unbounded output risks context overflow,
// OOM and runaway transcript files. Tools with their own tighter caps (http /
// shell / fs) stay well under this ceiling; this is the backstop for everything
// else — notably MCP tools, whose output size we do not control. Process-global and
// settings-driven (MaxToolOutputKB) via SetMaxToolOutputBytes, pushed from applySettings.
var maxToolOutputBytes = 100 * 1024

// SetMaxToolOutputBytes overrides the tool-output backstop cap (in bytes). A value
// <= 0 leaves it unchanged, so a partial settings push never zeroes the cap.
func SetMaxToolOutputBytes(n int) {
	if n > 0 {
		maxToolOutputBytes = n
	}
}

// offloadHeadBudget is the share of maxToolOutputBytes spent on the head of an
// offloaded output; the remainder carries the tail. A head-only truncation always
// loses the ending — exit status, final rows, the error trailer — which is usually
// the most decisive part of a long tool output, so the offload keeps both ends.
const offloadHeadBudget = 0.7

// truncHead returns the longest valid-UTF-8 prefix of s that fits in n bytes.
func truncHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// truncTail returns the longest valid-UTF-8 suffix of s that fits in n bytes.
func truncTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[len(s)-n:]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[1:]
	}
	return cut
}

// capToolOutput truncates s to maxToolOutputBytes on a UTF-8 boundary and
// appends a marker when it overflows, so the model is told output was cut.
//
// The cap bounds the KEPT BODY, not the returned string: the marker is added on
// top, so the result can be up to maxToolOutputBytes + len(marker) (~27 bytes).
// That is deliberate — the cap is a context/OOM backstop, and no consumer of
// this value treats it as a hard ceiling (the tool loop itself appends
// guardrail and repair hints to the result afterwards). Do not "fix" the
// overshoot by shrinking the body; a caller that truly needs a hard bound must
// enforce it on its own side.
func capToolOutput(s string) string {
	if len(s) <= maxToolOutputBytes {
		return s
	}
	cut := truncHead(s, maxToolOutputBytes)
	return fmt.Sprintf("%s\n…[truncated %d bytes]", cut, len(s)-len(cut))
}

// capToolOutputOffload bounds a tool's output like capToolOutput, but when an
// artifact sink is attached to ctx the FULL output is first persisted as a text
// artifact and the model receives head + tail + an artifact handle. The dropped
// middle then stays retrievable instead of being destroyed by the cut.
//
// Without a sink (a turn with no session, or the claude-cli bridge registry) the
// behaviour is byte-for-byte capToolOutput: no sink means no offload, and plain
// truncation still applies.
//
// A failed artifact write is logged and ALSO falls back to plain truncation, on
// purpose: the model still needs this tool's result, and the output was already
// going to be truncated before this feature existed. Turning a storage problem
// into a failed tool call would be a strict regression, so the failure degrades
// the output rather than the call.
//
// Same cap semantics as capToolOutput: head + tail together are exactly
// maxToolOutputBytes and the artifact marker (~110 bytes, its length varies with
// the artifact id) rides on top, so the returned string overshoots the cap by
// ~0.1%. Intentional, for the reason spelled out on capToolOutput.
func capToolOutputOffload(ctx context.Context, toolName, s string) string {
	if len(s) <= maxToolOutputBytes {
		return s
	}
	sink := artifactsFrom(ctx)
	if sink == nil {
		return capToolOutput(s)
	}
	art, err := sink.CreateArtifact(ctx, CreateArtifactSpec{
		Title:   fmt.Sprintf("%s — full output (%d bytes)", toolName, len(s)),
		Kind:    "text",
		Content: s,
	})
	if err != nil {
		slog.Warn("tool output offload failed; falling back to plain truncation",
			"component", "tools", "tool", toolName, "bytes", len(s), "error", err)
		return capToolOutput(s)
	}
	headBudget := int(float64(maxToolOutputBytes) * offloadHeadBudget)
	head := truncHead(s, headBudget)
	tail := truncTail(s, maxToolOutputBytes-len(head))
	return fmt.Sprintf(
		"%s\n…[%d bytes elided — full output saved as 📎 artifact %s; open it if you need the middle]…\n%s",
		head, len(s)-len(head)-len(tail), art.ID, tail)
}

// Tool is an in-process (built-in) tool.
type Tool interface {
	Def() providers.ToolDef
	Call(ctx context.Context, input json.RawMessage) (string, error)
}

// StreamingTool is an optional interface a built-in tool may implement to stream
// its output incrementally while it runs. onChunk is called with each chunk as
// it is produced (may be nil — then it behaves like Call); the full output is
// still returned for the model.
type StreamingTool interface {
	Tool
	CallStream(ctx context.Context, input json.RawMessage, onChunk func(string)) (string, error)
}

// Registry aggregates built-in tools and an MCP catalog for one agent context.
type Registry struct {
	builtins map[string]Tool
	// lazy is the set of tool names whose schema is loaded on demand (see
	// ToolDef.Lazy). Lazy tools are omitted from ActiveDefs until activated, but
	// always listed by LazyCatalog and always returned by Defs (full catalog).
	lazy map[string]bool
	// hidden is the subset of lazy tools kept OUT of the rendered "Available Tools
	// (load on demand)" prompt block — activatable and searchable, but not
	// enumerated in the cached static prefix. The self-management suite lives here:
	// its catalog is documented in the `tionharness-self-management` skill instead, so
	// dozens of summaries don't ride in every turn's prompt. hidden ⊆ lazy.
	hidden map[string]bool
	// nameOnly is the subset of lazy tools rendered in the load-on-demand catalog
	// as NAME ONLY (summary suppressed) — the Claude Code "deferred tool" style.
	// They stay listed (unlike hidden, which is dropped entirely), so the model
	// sees the name and discovers what it does via tool_search before activating.
	// nameOnly ⊆ lazy and is disjoint from hidden. The workspace tools screen's
	// "NameOnly" chip maps here.
	nameOnly map[string]bool
	// selfManaged marks built-ins that belong to the self-management suite — a
	// STABLE membership set stamped at registration, independent of the visibility
	// marks above (which a per-workspace override can flip). The claude-cli bridge
	// uses it to keep advertising a self-management tool even when the user promotes
	// it to the full tier (which clears its lazy flag): without this, a full-tier
	// self-management tool would vanish from the CLI entirely (BridgeableDefs bridges
	// lazy built-ins only). See BridgeableDefsFiltered.
	selfManaged map[string]bool
	active      *ActiveTools
	allow       func(string) bool

	mcpEntries     []mcp.CatalogEntry
	mcpCfgByServer map[string]mcp.ServerConfig
	// mcpCaller dispatches a namespaced MCP tool call. When set (the production
	// path) it routes through the workspace's persistent connection pool so a
	// server's session state (e.g. the gateway's activate_tools) survives across
	// calls. Nil falls back to dial-per-call (used by lightweight tests).
	mcpCaller MCPCaller
}

// ConfigureAutoActivation wires the per-turn activation state and effective
// permission filter used to recover direct calls to deferred tools.
func (r *Registry) ConfigureAutoActivation(active *ActiveTools, allow func(string) bool) {
	r.active = active
	r.allow = allow
}

// MCPCaller invokes a namespaced MCP tool and returns its flattened result.
type MCPCaller func(ctx context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error)

// NewRegistry creates a registry seeded with the given built-in tools.
func NewRegistry(builtins ...Tool) *Registry {
	r := &Registry{
		builtins:       map[string]Tool{},
		lazy:           map[string]bool{},
		hidden:         map[string]bool{},
		nameOnly:       map[string]bool{},
		selfManaged:    map[string]bool{},
		mcpCfgByServer: map[string]mcp.ServerConfig{},
	}
	for _, t := range builtins {
		r.builtins[t.Def().Name] = t
	}
	return r
}

// Add registers extra built-in tools after construction (e.g. the activate_tools
// meta-tool, which needs the lazy catalog assembled from earlier tools + MCP).
func (r *Registry) Add(extra ...Tool) {
	for _, t := range extra {
		r.builtins[t.Def().Name] = t
	}
}

// MarkLazy flags the named tools as lazy (loaded on demand via activate_tools).
func (r *Registry) MarkLazy(names ...string) {
	for _, n := range names {
		r.lazy[n] = true
	}
}

// MarkHidden flags the named tools as hidden-lazy: lazy (loaded on demand) AND
// omitted from the rendered load-on-demand catalog block. They stay activatable
// (activate_tools) and searchable (tool_search) — only their per-turn prompt
// enumeration is dropped. Implies MarkLazy.
func (r *Registry) MarkHidden(names ...string) {
	for _, n := range names {
		r.lazy[n] = true
		r.hidden[n] = true
		delete(r.nameOnly, n) // hidden and nameOnly are disjoint; last mark wins
	}
}

// MarkNameOnly flags lazy tools to render in the load-on-demand catalog as NAME
// ONLY (summary suppressed), the Claude Code "deferred tool" style. They stay
// listed (the model sees the name), activatable (activate_tools) and searchable
// (tool_search) — only the per-turn summary line is dropped. Implies MarkLazy.
// The workspace tools screen's "NameOnly" chip maps here.
func (r *Registry) MarkNameOnly(names ...string) {
	for _, n := range names {
		r.lazy[n] = true
		r.nameOnly[n] = true
		delete(r.hidden, n) // nameOnly and hidden are disjoint; last mark wins
	}
}

// MarkSelfManaged records the named tools as members of the self-management suite.
// This is a stable membership stamp (not a visibility tier), set once at build time
// so the claude-cli bridge can still advertise a self-management tool after the user
// promotes it to the full tier (which clears its lazy flag). Idempotent; unknown
// names are harmless.
func (r *Registry) MarkSelfManaged(names ...string) {
	for _, n := range names {
		r.selfManaged[n] = true
	}
}

// IsSelfManaged reports whether a tool is a member of the self-management suite.
func (r *Registry) IsSelfManaged(name string) bool { return r.selfManaged[name] }

// Unlazy forces the named tools eager (shipped every turn): it clears any lazy,
// hidden AND name-only marks, overriding code defaults like the self-management
// suite's MarkHidden. Used by the workspace "show" override so a user can surface
// an otherwise-hidden tool. Unknown names are harmless no-ops.
func (r *Registry) Unlazy(names ...string) {
	for _, n := range names {
		delete(r.lazy, n)
		delete(r.hidden, n)
		delete(r.nameOnly, n)
	}
}

// Visibility tiers describe how much of a tool rides in the per-turn context.
// Exactly ONE applies to a tool at a time; they are the user-selectable states on
// the workspace tools screen and map directly onto the registry's lazy/nameOnly/
// hidden marks. See _Docs/19.
const (
	// VisibilityFull: eager — the full schema is shipped to the model every turn.
	VisibilityFull = "full"
	// VisibilitySummary: lazy — listed in the load-on-demand catalog with name + a
	// short summary line; the full schema is pulled on demand (activate_tools).
	VisibilitySummary = "summary"
	// VisibilityNameOnly: lazy — listed in the catalog by NAME ALONE (summary
	// suppressed), Claude Code deferred-tool style. Discovered via tool_search.
	VisibilityNameOnly = "name-only"
	// VisibilityHidden: lazy — folded OUT of the catalog entirely (not even named),
	// reachable only via tool_search / the self-management skill pointer.
	VisibilityHidden = "hidden"
)

// SetVisibility forces a tool into exactly one visibility tier, clearing the other
// (mutually exclusive) tier marks first. An unknown tier is a no-op returning
// false. An unknown tool name is harmless — marks are applied lazily at render.
func (r *Registry) SetVisibility(name, tier string) bool {
	switch tier {
	case VisibilityFull:
		r.Unlazy(name) // clears lazy + nameOnly + hidden → eager
	case VisibilitySummary:
		r.lazy[name] = true
		delete(r.nameOnly, name)
		delete(r.hidden, name)
	case VisibilityNameOnly:
		r.MarkNameOnly(name) // sets lazy + nameOnly, clears hidden
	case VisibilityHidden:
		r.MarkHidden(name) // sets lazy + hidden, clears nameOnly
	default:
		return false
	}
	return true
}

// VisibilityOf reports a tool's current effective visibility tier, derived from
// its lazy/hidden/nameOnly marks (the inverse of SetVisibility).
func (r *Registry) VisibilityOf(name string) string {
	if !r.lazy[name] {
		return VisibilityFull
	}
	if r.hidden[name] {
		return VisibilityHidden
	}
	if r.nameOnly[name] {
		return VisibilityNameOnly
	}
	return VisibilitySummary
}

// IsLazy reports whether a tool is lazy.
func (r *Registry) IsLazy(name string) bool { return r.lazy[name] }

// EagerNames returns the set of built-in tools that ship EVERY turn (not lazy) —
// i.e. those already callable without activate_tools. allow filters by name (nil
// = allow all). activate_tools consults this so a request to activate an already
// eager tool (e.g. todo_write) reports "already available" rather than the
// confusing "unknown name". MCP tools are omitted: they are lazy by default and
// any eager MCP tool is still callable, so a stray activation of one is harmless
// and the lazy-catalog path already covers the tools an agent needs to activate.
func (r *Registry) EagerNames(allow func(name string) bool) map[string]bool {
	out := make(map[string]bool)
	for name := range r.builtins {
		if r.lazy[name] {
			continue
		}
		if allow == nil || allow(name) {
			out[name] = true
		}
	}
	return out
}

// IsHidden reports whether a tool is in the HIDDEN tier: lazy AND folded out of
// the per-turn catalog into the self-management skill pointer (not enumerated by
// name). Distinct from a plain name-only tool, which stays listed by name.
func (r *Registry) IsHidden(name string) bool { return r.hidden[name] }

// AttachMCP records the MCP catalog and per-server configs so the registry can
// advertise and dispatch namespaced MCP tools. Every MCP tool is marked lazy AND
// name-only: external servers can expose hundreds of tools, so their schemas are
// loaded on demand rather than shipped every turn, and their (often multi-
// paragraph) descriptions are suppressed in the load-on-demand catalog — the model
// sees only the namespaced name and discovers the rest via tool_search/ToolSearch.
// This mirrors TionHarness's own deferred (tionharness_extended) tools: no deferred tool
// carries a full description in the per-turn prompt.
func (r *Registry) AttachMCP(entries []mcp.CatalogEntry, cfgByServer map[string]mcp.ServerConfig, caller MCPCaller) {
	r.mcpEntries = entries
	r.mcpCfgByServer = cfgByServer
	r.mcpCaller = caller
	// The name-only tier is not hardcoded here: it comes from the bundle-default
	// table's "mcp:*" row (tierdefaults.go), so a later layer can demote or promote
	// a single server without touching this call site.
	defaults := DefaultTiers()
	for _, e := range entries {
		r.lazy[e.NamespacedName] = true
		if tier := defaults.TierFor(e.NamespacedName); tier != "" {
			r.SetVisibility(e.NamespacedName, tier)
		}
	}
}

// ServerDescriptions returns the curated one-liner for each attached MCP server
// (server name → description), skipping servers without one. Used to enrich the
// per-server summary line of the load-on-demand catalog so a semantic hint
// survives even when a workspace has too many MCP tools to enumerate.
func (r *Registry) ServerDescriptions() map[string]string {
	if len(r.mcpCfgByServer) == 0 {
		return nil
	}
	out := make(map[string]string, len(r.mcpCfgByServer))
	for name, cfg := range r.mcpCfgByServer {
		if d := strings.TrimSpace(cfg.Description); d != "" {
			out[name] = d
		}
	}
	return out
}

// foldExamples merges a tool's Examples into its InputSchema as a JSON Schema
// "examples" array, so sample calls travel with the FULL schema sent to the model.
// Tools without examples (or without an object schema) are returned unchanged.
// Deliberately NOT applied by LazyCatalog, which emits name+description only — so
// examples never bloat the load-on-demand catalog, only the activated schema.
func foldExamples(d providers.ToolDef) providers.ToolDef {
	if len(d.Examples) == 0 || len(d.InputSchema) == 0 {
		return d
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(d.InputSchema, &obj); err != nil {
		return d // non-object schema — leave as-is
	}
	arr, err := json.Marshal(d.Examples)
	if err != nil {
		return d
	}
	obj["examples"] = arr
	merged, err := json.Marshal(obj)
	if err != nil {
		return d
	}
	d.InputSchema = merged
	return d
}

// foldStrict normalizes a Strict-marked tool's schema for API-side validation:
// strict tool use requires additionalProperties:false AND a required array on
// the root object, so both are injected when absent (empty required = all
// params optional). Non-strict tools pass through untouched.
func foldStrict(d providers.ToolDef) providers.ToolDef {
	if !d.Strict || len(d.InputSchema) == 0 {
		return d
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(d.InputSchema, &obj); err != nil {
		d.Strict = false // non-object schema can't be strict-validated
		return d
	}
	changed := false
	if _, ok := obj["additionalProperties"]; !ok {
		obj["additionalProperties"] = json.RawMessage("false")
		changed = true
	}
	if _, ok := obj["required"]; !ok {
		obj["required"] = json.RawMessage("[]")
		changed = true
	}
	if changed {
		if merged, err := json.Marshal(obj); err == nil {
			d.InputSchema = merged
		}
	}
	return d
}

// prepDef applies the request-assembly normalizations to a builtin def.
func prepDef(d providers.ToolDef) providers.ToolDef { return foldStrict(foldExamples(d)) }

// Defs returns the tool schemas to offer the model. If allow is non-nil, only
// tools whose name satisfies allow(name) are included.
func (r *Registry) Defs(allow func(name string) bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if allow == nil || allow(name) {
			out = append(out, prepDef(t.Def()))
		}
	}
	for _, e := range r.mcpEntries {
		if allow != nil && !allow(e.NamespacedName) {
			continue
		}
		out = append(out, providers.ToolDef{
			Name:        e.NamespacedName,
			Description: e.Tool.Description,
			// External MCP schemas are normalized before they reach the provider:
			// $schema/draft plumbing stripped, root object guaranteed (CG-4).
			InputSchema: mcp.NormalizeSchema(e.Tool.InputSchema),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ActiveDefs returns the tool schemas to ship to the model for a turn: every
// EAGER tool (not lazy) plus the lazy tools the model has ACTIVATED (active set).
// allow filters by name as in Defs (nil = allow all). This is the request-time
// counterpart of Defs, which always returns the full catalog.
func (r *Registry) ActiveDefs(allow func(name string) bool, active map[string]bool) []providers.ToolDef {
	keep := func(name string) bool {
		if allow != nil && !allow(name) {
			return false
		}
		return !r.lazy[name] || active[name]
	}
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if keep(name) {
			out = append(out, prepDef(t.Def()))
		}
	}
	for _, e := range r.mcpEntries {
		if keep(e.NamespacedName) {
			out = append(out, providers.ToolDef{
				Name:        e.NamespacedName,
				Description: e.Tool.Description,
				InputSchema: mcp.NormalizeSchema(e.Tool.InputSchema),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DeferredDefs returns the FULL tool catalog for providers with NATIVE
// (server-side) tool search: eager tools ship normally; every lazy tool —
// summary, name-only and hidden tiers alike, plus all MCP tools — is included
// with DeferLoading set, so the server indexes it for regex discovery without
// spending context tokens until the model actually finds it. Tools the model
// has explicitly ACTIVATED this turn (activate_tools) ship non-deferred so they
// stay hot. allow filters by name as in Defs (nil = allow all).
//
// Unlike ActiveDefs, the returned set is IDENTICAL every iteration (activation
// aside), so the request's tools block stays byte-stable across the loop — the
// cache-friendliest shape. TionHarness's own activate_tools/tool_search builtins
// remain in the set and keep working; native search is an additional, round-trip
// -free discovery path on top.
func (r *Registry) DeferredDefs(allow func(name string) bool, active map[string]bool) []providers.ToolDef {
	var out []providers.ToolDef
	add := func(d providers.ToolDef, lazy bool) {
		if allow != nil && !allow(d.Name) {
			return
		}
		d.DeferLoading = lazy && !active[d.Name]
		out = append(out, d)
	}
	for name, t := range r.builtins {
		add(prepDef(t.Def()), r.lazy[name])
	}
	for _, e := range r.mcpEntries {
		add(providers.ToolDef{
			Name:        e.NamespacedName,
			Description: e.Tool.Description,
			InputSchema: mcp.NormalizeSchema(e.Tool.InputSchema),
		}, r.lazy[e.NamespacedName])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// lazyCatalogDescMaxChars bounds a single tool's description in the rendered
// "Available Tools (load on demand)" block. The lazy block is a name+summary
// teaser only — the FULL description ships later, in the activated tool's schema
// (Defs). MCP servers (notably the gateway) attach multi-paragraph descriptions
// full of "When to use / When NOT to use" guidance; dumping all of that for every
// lazy tool bloats the system prompt (and on Windows pushes the claude-cli
// command line past the ~32 KB limit). We keep just the first meaningful line.
const lazyCatalogDescMaxChars = 200

// lazyDescription reduces a (possibly multi-paragraph) tool description to a
// single short summary line for the load-on-demand catalog. It takes the first
// non-empty line and hard-caps its length on a UTF-8 boundary.
func lazyDescription(desc string) string {
	summary := ""
	for _, line := range strings.Split(desc, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			summary = s
			break
		}
	}
	if utf8.RuneCountInString(summary) <= lazyCatalogDescMaxChars {
		return summary
	}
	runes := []rune(summary)
	return strings.TrimSpace(string(runes[:lazyCatalogDescMaxChars])) + "…"
}

// LazyCatalog returns the lazy tools (name + short summary) for the
// load-on-demand prompt block. allow filters by name (nil = allow all).
func (r *Registry) LazyCatalog(allow func(name string) bool) []providers.ToolDef {
	keep := func(name string) bool {
		return r.lazy[name] && (allow == nil || allow(name))
	}
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if keep(name) {
			d := t.Def()
			out = append(out, providers.ToolDef{Name: d.Name, Description: lazyDescription(d.Description)})
		}
	}
	for _, e := range r.mcpEntries {
		if keep(e.NamespacedName) {
			out = append(out, providers.ToolDef{Name: e.NamespacedName, Description: lazyDescription(e.Tool.Description)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// VisibleLazyCatalog is LazyCatalog minus hidden tools — the set actually
// enumerated in the load-on-demand prompt block. Hidden tools (the self-
// management suite) are excluded here but remain in LazyCatalog (so they stay
// activatable/searchable). allow filters by name (nil = allow all).
func (r *Registry) VisibleLazyCatalog(allow func(name string) bool) []providers.ToolDef {
	full := r.LazyCatalog(allow)
	out := full[:0:0]
	for _, d := range full {
		if r.hidden[d.Name] {
			continue
		}
		// NameOnly tools stay listed but shed their summary — the renderer emits
		// just the backticked name (Claude Code deferred-tool style).
		if r.nameOnly[d.Name] {
			d.Description = ""
		}
		out = append(out, d)
	}
	return out
}

// HiddenLazyCount returns how many lazy tools are hidden from the prompt block
// (after the allow filter) — used to decide whether to render the skill pointer.
func (r *Registry) HiddenLazyCount(allow func(name string) bool) int {
	n := 0
	for name := range r.builtins {
		if r.lazy[name] && r.hidden[name] && (allow == nil || allow(name)) {
			n++
		}
	}
	return n
}

// bridgeExcluded names lazy built-ins that must NOT be advertised to the
// claude-cli Interaction MCP bridge, even though they are lazy and would
// otherwise qualify. Two reasons, both about keeping the CLI surface lean and
// correct:
//   - WebFetch  : the CLI already has its own native WebFetch — bridging ours
//     only doubles the schema cost (the bridge ships FULL schemas, unlike the
//     native lazy catalog which ships name+summary only).
//   - run_subagent: its Call needs the run-agent runner from context, which the
//     generic bridge dispatcher (plain request context) cannot supply. It is
//     instead bridged explicitly via interactionToolSpecs + callRunSubagent,
//     which install a per-run runner — so CLI agents DO get synchronous
//     delegation, just not through this generic lazy-built-in path.
//
// Native agents are unaffected — they reach these through activate_tools as usual.
var bridgeExcluded = map[string]bool{
	"WebFetch":     true,
	"run_subagent": true,
	// run_code (code-execution mode, _Docs/44) is native-path-only: the CLI has
	// its own Bash + ToolSearch story, and the tool is eager anyway — this entry
	// is a defensive guard in case a workspace override ever marks it lazy.
	"run_code": true,
}

// BridgeableDefs returns the FULL schemas of lazy built-in tools — the
// self-management family — so the CLI path (Interaction MCP bridge) can advertise
// and call them. MCP tools are excluded (the CLI reaches those through their own
// server entries); only in-process built-ins are bridged. Tools in bridgeExcluded
// are skipped (CLI-native or native-loop-context-bound). allow filters by name
// (nil = allow all), mirroring the per-agent tool filter used on the native path.
func (r *Registry) BridgeableDefs(allow func(name string) bool) []providers.ToolDef {
	// Delegates to the filtered form with skipHidden=false, preserving the
	// historical behaviour (hidden-tier lazy tools ARE bridged to the CLI with
	// full schemas). See bridge_filter.go for the POC hidden-tier exclusion.
	return r.BridgeableDefsFiltered(allow, false)
}

// BuiltinDefs returns the ToolDefs of the registry's in-process built-in tools
// (NOT MCP tools). Used by code-execution mode to expose eligible built-ins as
// Python bindings. allow filters by name (nil = allow all).
func (r *Registry) BuiltinDefs(allow func(name string) bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if allow == nil || allow(name) {
			out = append(out, t.Def())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Has reports whether the registry knows a tool by (namespaced) name.
func (r *Registry) Has(name string) bool {
	if _, ok := r.builtins[name]; ok {
		return true
	}
	for _, e := range r.mcpEntries {
		if e.NamespacedName == name {
			return true
		}
	}
	return false
}

// MCPSchema returns the declared input schema of an attached MCP tool, addressed
// by its namespaced name, or nil when the name is not an attached MCP tool (a
// built-in, or unknown). Callers use it to validate a model-authored argument
// object BEFORE it reaches the server, so a missing required field surfaces as
// an accurate local error instead of whatever the server infers from it.
func (r *Registry) MCPSchema(name string) json.RawMessage {
	for _, e := range r.mcpEntries {
		if e.NamespacedName == name {
			return e.Tool.InputSchema
		}
	}
	return nil
}

// Empty reports whether there are no tools at all.
func (r *Registry) Empty() bool { return len(r.builtins) == 0 && len(r.mcpEntries) == 0 }

// Call executes a tool call and returns a result to feed back to the model.
// Errors are returned as IsError results (not Go errors) so the agentic loop
// can hand the failure to the model rather than aborting.
func (r *Registry) Call(ctx context.Context, call providers.ToolCall) providers.ToolResult {
	res := providers.ToolResult{CallID: call.ID}
	name := r.resolveCatalogName(call.Name)
	if name != "" && r.allow != nil && !r.allow(name) {
		res.Content = fmt.Sprintf("tool %q exists but is disabled or not permitted in this workspace", name)
		res.IsError = true
		return res
	}
	// A nil active set means this registry does not track activation at all (the
	// claude-cli bridge registry built by Runtime.BridgeTools). There the gate lives
	// in the Interaction layer (isActivated), so applying a second gate here would
	// make every bridged on-demand tool permanently uncallable.
	if name != "" && r.lazy[name] && r.active != nil && !r.active.Has(name) {
		if r.hidden[name] {
			res.Content = fmt.Sprintf("tool %q exists but is disabled or not permitted in this workspace", name)
			res.IsError = true
			return res
		}
		if r.active != nil && r.active.AutoActivate(name) {
			res.Content = fmt.Sprintf("Tool %q was activated automatically. Its schema is now available; reissue the same call using the schema on your next step.", name)
			res.IsError = true
			return res
		}
		res.Content = fmt.Sprintf("tool %q is deferred and cannot be called before its schema is available", name)
		res.IsError = true
		return res
	}
	if name != "" {
		call.Name = name
	}

	if t, ok := r.builtins[call.Name]; ok {
		out, err := t.Call(ctx, call.Input)
		if err != nil {
			res.Content = "tool error: " + err.Error()
			res.IsError = true
			return res
		}
		res.Content = capToolOutputOffload(ctx, call.Name, out)
		return res
	}

	if _, _, ok := mcp.SplitNamespaced(call.Name); ok && r.Has(call.Name) {
		var out mcp.CallToolResult
		var err error
		if r.mcpCaller != nil {
			out, err = r.mcpCaller(ctx, call.Name, call.Input)
		} else {
			out, err = mcp.CallNamespaced(ctx, r.mcpCfgByServer, call.Name, call.Input)
		}
		if err != nil {
			// Transport/dial failures would otherwise only surface as an IsError
			// tool result fed back to the model — invisible in the Logs screen.
			slog.Warn("mcp tool call failed", "component", "mcp", "tool", call.Name, "error", err)
			res.Content = "mcp tool error: " + err.Error()
			res.IsError = true
			return res
		}
		res.Content = capToolOutputOffload(ctx, call.Name, out.Text)
		res.IsError = out.IsError
		return res
	}

	res.Content = r.unknownToolMessage(call.Name)
	res.IsError = true
	return res
}

func (r *Registry) resolveCatalogName(name string) string {
	if r.Has(name) {
		return name
	}
	bare := name
	if i := strings.LastIndex(name, "__"); i >= 0 {
		bare = name[i+2:]
	}
	var match string
	for _, d := range r.Defs(nil) {
		candidate := d.Name
		if i := strings.LastIndex(candidate, "__"); i >= 0 {
			candidate = candidate[i+2:]
		}
		if candidate != bare {
			continue
		}
		if match != "" {
			return ""
		}
		match = d.Name
	}
	return match
}

func (r *Registry) unknownToolMessage(name string) string {
	needle := strings.ToLower(name)
	var nearby []string
	for _, d := range r.Defs(nil) {
		candidate := strings.ToLower(d.Name)
		if strings.Contains(candidate, needle) || strings.Contains(needle, candidate) || toolNameDistance(candidate, needle) <= 3 {
			nearby = append(nearby, d.Name)
		}
	}
	sort.Strings(nearby)
	msg := fmt.Sprintf("unknown tool %q; use tool_search to find the exact tool name", name)
	if len(nearby) > 0 {
		msg += "; nearby tools: " + strings.Join(nearby, ", ")
	}
	return msg
}

func toolNameDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

// CanStream reports whether the named built-in tool streams its output.
func (r *Registry) CanStream(name string) bool {
	t, ok := r.builtins[name]
	if !ok {
		return false
	}
	_, ok = t.(StreamingTool)
	return ok
}

// CallStream executes a tool call, forwarding incremental output to onChunk when
// the built-in supports streaming; otherwise it behaves exactly like Call.
func (r *Registry) CallStream(ctx context.Context, call providers.ToolCall, onChunk func(string)) providers.ToolResult {
	if t, ok := r.builtins[call.Name]; ok {
		if st, sok := t.(StreamingTool); sok && onChunk != nil {
			res := providers.ToolResult{CallID: call.ID}
			out, err := st.CallStream(ctx, call.Input, onChunk)
			if err != nil {
				res.Content = "tool error: " + err.Error()
				res.IsError = true
				return res
			}
			res.Content = capToolOutputOffload(ctx, call.Name, out)
			return res
		}
	}
	return r.Call(ctx, call)
}
