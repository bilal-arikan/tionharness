package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// lazyEntry is one row of the load-on-demand tool catalog (name + summary).
type lazyEntry struct {
	name string
	desc string
}

func toLazyEntries(catalog []providers.ToolDef) []lazyEntry {
	out := make([]lazyEntry, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, lazyEntry{name: d.Name, desc: d.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// ---- activate_tools -------------------------------------------------------

// ActivateToolsTool loads the full schemas of lazy tools on demand: the model
// names the tools it wants, they enter the active set, and their real schemas
// are shipped on the next iteration so it can call them.
type ActivateToolsTool struct {
	active *ActiveTools
	byName map[string]string // lazy tool name -> description
	eager  map[string]bool   // always-on tool names (already callable, no activation needed)
}

// NewActivateToolsTool builds the tool over the active set, the lazy catalog and
// the set of eager (always-on) tool names. The eager set lets the tool answer a
// request to activate an already-shipped tool with a clear "already available"
// note instead of the misleading "unknown name" (eager tools are never in the
// lazy catalog). A nil eager set is fine — such names simply fall back to unknown.
func NewActivateToolsTool(active *ActiveTools, catalog []providers.ToolDef, eager map[string]bool) ActivateToolsTool {
	byName := make(map[string]string, len(catalog))
	for _, e := range toLazyEntries(catalog) {
		byName[e.name] = e.desc
	}
	return ActivateToolsTool{active: active, byName: byName, eager: eager}
}

func (ActivateToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "activate_tools",
		Description: "Load the full schemas of one or more tools listed in your system prompt under " +
			"\"Available Tools (load on demand)\". Call this BEFORE using such a tool: pass its exact " +
			"name(s). Activate everything you expect to need for the task in one call. IMPORTANT: the " +
			"activated tools only become callable on your NEXT step — do NOT call them in the SAME " +
			"response/batch as this activate_tools call, or the runtime will reject them as \"No such " +
			"tool available\". Activate now, use them next turn.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "names": { "type": "array", "items": { "type": "string" }, "description": "Exact tool names to activate." }
  },
  "required": ["names"],
  "additionalProperties": false
}`),
	}
}

// resolveLazyName maps a requested tool name to a real catalog name, tolerating
// the common namespace confusion where the model prefixes a name with an
// mcp__<server>__ (or <server>__) segment it invented — or drops one an MCP tool
// actually has. An EXACT match always wins. Otherwise it compares by the final
// "__"-segment (the bare tool name) and returns the catalog entry only when that
// match is UNAMBIGUOUS (exactly one), so a wrong guess is never silently routed
// to the wrong tool. "" means no confident match (caller reports it as unknown).
func (t ActivateToolsTool) resolveLazyName(n string) string {
	if _, ok := t.byName[n]; ok {
		return n
	}
	bare := func(s string) string {
		if i := strings.LastIndex(s, "__"); i >= 0 {
			return s[i+2:]
		}
		return s
	}
	want := bare(n)
	var hits []string
	for name := range t.byName {
		if name == want || bare(name) == want {
			hits = append(hits, name)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

// resolveEagerName maps a requested name to an always-on (eager) tool, tolerating
// the same invented-namespace confusion as resolveLazyName. An EXACT match wins;
// otherwise it matches on the bare final "__"-segment when UNAMBIGUOUS. "" means
// the name is not an eager tool. Used only to turn "activate an already-shipped
// tool" into a helpful note rather than a misleading "unknown name".
func (t ActivateToolsTool) resolveEagerName(n string) string {
	if t.eager[n] {
		return n
	}
	bare := func(s string) string {
		if i := strings.LastIndex(s, "__"); i >= 0 {
			return s[i+2:]
		}
		return s
	}
	want := bare(n)
	var hits []string
	for name := range t.eager {
		if name == want || bare(name) == want {
			hits = append(hits, name)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

func (t ActivateToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("activate_tools", err)
	}
	var known, alwaysOn, unknown []string
	for _, n := range in.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if r := t.resolveLazyName(n); r != "" {
			known = append(known, r)
		} else if r := t.resolveEagerName(n); r != "" {
			alwaysOn = append(alwaysOn, r)
		} else {
			unknown = append(unknown, n)
		}
	}
	if len(known) == 0 {
		var b strings.Builder
		if len(alwaysOn) > 0 {
			fmt.Fprintf(&b, "Nothing to activate: %s already available (always-on) — just call it directly.\n", strings.Join(alwaysOn, ", "))
		}
		if len(unknown) > 0 {
			fmt.Fprintf(&b, "Unknown names: %s. Use the exact names from the \"Available Tools (load on demand)\" list (or tool_search).\n", strings.Join(unknown, ", "))
		}
		if b.Len() == 0 {
			return "No tool names given.", nil
		}
		return strings.TrimSpace(b.String()), nil
	}
	added, already := t.active.Activate(known...)

	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "Activated %d tool(s). Their schemas arrive on your NEXT step — do not call them in this same response:\n", len(added))
		for _, n := range added {
			fmt.Fprintf(&b, "- %s — %s\n", n, t.byName[n])
		}
	}
	if len(already) > 0 {
		fmt.Fprintf(&b, "Already active: %s\n", strings.Join(already, ", "))
	}
	if len(alwaysOn) > 0 {
		fmt.Fprintf(&b, "Already available (always-on, no activation needed): %s\n", strings.Join(alwaysOn, ", "))
	}
	if len(unknown) > 0 {
		fmt.Fprintf(&b, "Unknown (skipped): %s\n", strings.Join(unknown, ", "))
	}
	return strings.TrimSpace(b.String()), nil
}

// ---- deactivate_tools -----------------------------------------------------

// DeactivateToolsTool drops tools from the active set to keep the shipped schema
// set lean once they are no longer needed.
type DeactivateToolsTool struct {
	active *ActiveTools
}

// NewDeactivateToolsTool builds the tool over the active set.
func NewDeactivateToolsTool(active *ActiveTools) DeactivateToolsTool {
	return DeactivateToolsTool{active: active}
}

func (DeactivateToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "deactivate_tools",
		Description: "Remove previously activated on-demand tools you no longer need, so their schemas " +
			"stop being sent. Optional housekeeping; pass the exact tool name(s).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "names": { "type": "array", "items": { "type": "string" }, "description": "Exact tool names to deactivate." }
  },
  "required": ["names"],
  "additionalProperties": false
}`),
	}
}

func (t DeactivateToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("deactivate_tools", err)
	}
	removed := t.active.Deactivate(in.Names...)
	if len(removed) == 0 {
		return "No active tools matched; nothing deactivated.", nil
	}
	return "Deactivated: " + strings.Join(removed, ", "), nil
}

// ---- tool_search ----------------------------------------------------------

// ToolSearchTool searches the load-on-demand catalog by keyword, so the model can
// discover the right tool name in MCP-heavy workspaces with large catalogs. It is
// the keyword-search complement to the system-prompt catalog block: in MCP-heavy
// workspaces that block lists built-in tools in full but only summarises MCP tools
// per server, deferring individual discovery to this tool (mirrors the Anthropic
// "tool search" pattern — search instead of enumerate).
type ToolSearchTool struct {
	entries []lazyEntry
}

// NewToolSearchTool builds the tool over a snapshot of the lazy catalog.
func NewToolSearchTool(catalog []providers.ToolDef) ToolSearchTool {
	return ToolSearchTool{entries: toLazyEntries(catalog)}
}

func (ToolSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "tool_search",
		Description: "Search the on-demand tool catalog by keyword to find a tool's exact name before " +
			"activating it. Returns matching name — description lines. Use this when a tool you need is " +
			"not listed individually in the \"Available Tools (load on demand)\" block (e.g. MCP tools, " +
			"which are summarised per server in MCP-heavy workspaces).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keyword(s) to match against tool names and descriptions." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
	}
}

func (t ToolSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("tool_search", err)
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if q == "" {
		return "Empty query.", nil
	}
	terms := strings.Fields(q)
	var matches []string
	for _, e := range t.entries {
		hay := strings.ToLower(e.name + " " + e.desc)
		all := true
		for _, term := range terms {
			if !strings.Contains(hay, term) {
				all = false
				break
			}
		}
		if all {
			matches = append(matches, fmt.Sprintf("- %s — %s", e.name, e.desc))
		}
	}
	if len(matches) == 0 {
		return fmt.Sprintf("No on-demand tools match %q.", in.Query), nil
	}
	const max = 30
	more := ""
	if len(matches) > max {
		more = fmt.Sprintf("\n…and %d more; refine the query.", len(matches)-max)
		matches = matches[:max]
	}
	return "Matching tools (activate with activate_tools):\n" + strings.Join(matches, "\n") + more, nil
}
