// Package tools provides the agent tool registry: a unified catalog of
// built-in (in-process) tools plus tools sourced from connected MCP servers.
// The registry produces provider.ToolDef schemas for a completion request and
// executes provider.ToolCall results, regardless of a tool's origin.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/bilal/swarmgo/internal/mcp"
	"github.com/bilal/swarmgo/internal/providers"
)

// maxToolOutputBytes bounds a tool's output before it is fed back to the model
// and persisted to the session JSONL. Unbounded output risks context overflow,
// OOM and runaway transcript files. Tools with their own tighter caps (http /
// shell / fs) stay well under this ceiling; this is the backstop for everything
// else — notably MCP tools, whose output size we do not control.
const maxToolOutputBytes = 100 * 1024

// capToolOutput truncates s to maxToolOutputBytes on a UTF-8 boundary and
// appends a marker when it overflows, so the model is told output was cut.
func capToolOutput(s string) string {
	if len(s) <= maxToolOutputBytes {
		return s
	}
	cut := s[:maxToolOutputBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return fmt.Sprintf("%s\n…[truncated %d bytes]", cut, len(s)-len(cut))
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

	mcpEntries     []mcp.CatalogEntry
	mcpCfgByServer map[string]mcp.ServerConfig
}

// NewRegistry creates a registry seeded with the given built-in tools.
func NewRegistry(builtins ...Tool) *Registry {
	r := &Registry{
		builtins:       map[string]Tool{},
		lazy:           map[string]bool{},
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

// IsLazy reports whether a tool is lazy.
func (r *Registry) IsLazy(name string) bool { return r.lazy[name] }

// AttachMCP records the MCP catalog and per-server configs so the registry can
// advertise and dispatch namespaced MCP tools. Every MCP tool is marked lazy:
// external servers can expose hundreds of tools, so their schemas are loaded on
// demand rather than shipped every turn.
func (r *Registry) AttachMCP(entries []mcp.CatalogEntry, cfgByServer map[string]mcp.ServerConfig) {
	r.mcpEntries = entries
	r.mcpCfgByServer = cfgByServer
	for _, e := range entries {
		r.lazy[e.NamespacedName] = true
	}
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

// Defs returns the tool schemas to offer the model. If allow is non-nil, only
// tools whose name satisfies allow(name) are included.
func (r *Registry) Defs(allow func(name string) bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if allow == nil || allow(name) {
			out = append(out, foldExamples(t.Def()))
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
			out = append(out, foldExamples(t.Def()))
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

// LazyCatalog returns the lazy tools (name + description only) for the
// load-on-demand prompt block. allow filters by name (nil = allow all).
func (r *Registry) LazyCatalog(allow func(name string) bool) []providers.ToolDef {
	keep := func(name string) bool {
		return r.lazy[name] && (allow == nil || allow(name))
	}
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if keep(name) {
			d := t.Def()
			out = append(out, providers.ToolDef{Name: d.Name, Description: d.Description})
		}
	}
	for _, e := range r.mcpEntries {
		if keep(e.NamespacedName) {
			out = append(out, providers.ToolDef{Name: e.NamespacedName, Description: e.Tool.Description})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// bridgeExcluded names lazy built-ins that must NOT be advertised to the
// claude-cli Interaction MCP bridge, even though they are lazy and would
// otherwise qualify. Two reasons, both about keeping the CLI surface lean and
// correct:
//   - http_get  : the CLI already has a native WebFetch — bridging it only
//     doubles the schema cost (the bridge ships FULL schemas, unlike the native
//     lazy catalog which ships name+summary only).
//   - call_agent: its Call needs the delegation runner from context
//     (DelegationFrom), which is installed only by the native tool loop. The
//     bridge dispatcher runs with a plain request context, so a bridged
//     call_agent would always fail with "delegation not available".
//
// Native agents are unaffected — they reach these through activate_tools as usual.
var bridgeExcluded = map[string]bool{
	"http_get":   true,
	"call_agent": true,
}

// BridgeableDefs returns the FULL schemas of lazy built-in tools — the
// self-management family — so the CLI path (Interaction MCP bridge) can advertise
// and call them. MCP tools are excluded (the CLI reaches those through their own
// server entries); only in-process built-ins are bridged. Tools in bridgeExcluded
// are skipped (CLI-native or native-loop-context-bound). allow filters by name
// (nil = allow all), mirroring the per-agent tool filter used on the native path.
func (r *Registry) BridgeableDefs(allow func(name string) bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if !r.lazy[name] || bridgeExcluded[name] {
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

// Empty reports whether there are no tools at all.
func (r *Registry) Empty() bool { return len(r.builtins) == 0 && len(r.mcpEntries) == 0 }

// Call executes a tool call and returns a result to feed back to the model.
// Errors are returned as IsError results (not Go errors) so the agentic loop
// can hand the failure to the model rather than aborting.
func (r *Registry) Call(ctx context.Context, call providers.ToolCall) providers.ToolResult {
	res := providers.ToolResult{CallID: call.ID}

	if t, ok := r.builtins[call.Name]; ok {
		out, err := t.Call(ctx, call.Input)
		if err != nil {
			res.Content = "tool error: " + err.Error()
			res.IsError = true
			return res
		}
		res.Content = capToolOutput(out)
		return res
	}

	if _, _, ok := mcp.SplitNamespaced(call.Name); ok {
		out, err := mcp.CallNamespaced(ctx, r.mcpCfgByServer, call.Name, call.Input)
		if err != nil {
			res.Content = "mcp tool error: " + err.Error()
			res.IsError = true
			return res
		}
		res.Content = capToolOutput(out.Text)
		res.IsError = out.IsError
		return res
	}

	res.Content = fmt.Sprintf("unknown tool %q", call.Name)
	res.IsError = true
	return res
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
			res.Content = capToolOutput(out)
			return res
		}
	}
	return r.Call(ctx, call)
}
