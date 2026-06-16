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

	"github.com/bilal/swarmgo/internal/mcp"
	"github.com/bilal/swarmgo/internal/providers"
)

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

	mcpEntries     []mcp.CatalogEntry
	mcpCfgByServer map[string]mcp.ServerConfig
}

// NewRegistry creates a registry seeded with the given built-in tools.
func NewRegistry(builtins ...Tool) *Registry {
	r := &Registry{
		builtins:       map[string]Tool{},
		mcpCfgByServer: map[string]mcp.ServerConfig{},
	}
	for _, t := range builtins {
		r.builtins[t.Def().Name] = t
	}
	return r
}

// AttachMCP records the MCP catalog and per-server configs so the registry can
// advertise and dispatch namespaced MCP tools.
func (r *Registry) AttachMCP(entries []mcp.CatalogEntry, cfgByServer map[string]mcp.ServerConfig) {
	r.mcpEntries = entries
	r.mcpCfgByServer = cfgByServer
}

// Defs returns the tool schemas to offer the model. If allow is non-nil, only
// tools whose name satisfies allow(name) are included.
func (r *Registry) Defs(allow func(name string) bool) []providers.ToolDef {
	var out []providers.ToolDef
	for name, t := range r.builtins {
		if allow == nil || allow(name) {
			out = append(out, t.Def())
		}
	}
	for _, e := range r.mcpEntries {
		if allow != nil && !allow(e.NamespacedName) {
			continue
		}
		out = append(out, providers.ToolDef{
			Name:        e.NamespacedName,
			Description: e.Tool.Description,
			InputSchema: e.Tool.InputSchema,
		})
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
		res.Content = out
		return res
	}

	if _, _, ok := mcp.SplitNamespaced(call.Name); ok {
		out, err := mcp.CallNamespaced(ctx, r.mcpCfgByServer, call.Name, call.Input)
		if err != nil {
			res.Content = "mcp tool error: " + err.Error()
			res.IsError = true
			return res
		}
		res.Content = out.Text
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
			res.Content = out
			return res
		}
	}
	return r.Call(ctx, call)
}
