package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
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
}

// NewActivateToolsTool builds the tool over the active set and the lazy catalog.
func NewActivateToolsTool(active *ActiveTools, catalog []providers.ToolDef) ActivateToolsTool {
	byName := make(map[string]string, len(catalog))
	for _, e := range toLazyEntries(catalog) {
		byName[e.name] = e.desc
	}
	return ActivateToolsTool{active: active, byName: byName}
}

func (ActivateToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "activate_tools",
		Description: "Load the full schemas of one or more tools listed in your system prompt under " +
			"\"Available Tools (load on demand)\". Call this BEFORE using such a tool: pass its exact " +
			"name(s); the tool(s) become callable on your next step. Activate everything you expect to " +
			"need for the task in one call.",
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

func (t ActivateToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid activate_tools input: %w", err)
	}
	var known, unknown []string
	for _, n := range in.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := t.byName[n]; ok {
			known = append(known, n)
		} else {
			unknown = append(unknown, n)
		}
	}
	if len(known) == 0 {
		if len(unknown) > 0 {
			return fmt.Sprintf("No tools activated. Unknown names: %s. Use the exact names from the \"Available Tools (load on demand)\" list (or find_tools).", strings.Join(unknown, ", ")), nil
		}
		return "No tool names given.", nil
	}
	added, already := t.active.Activate(known...)

	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "Activated %d tool(s); their schemas are available on your next step:\n", len(added))
		for _, n := range added {
			fmt.Fprintf(&b, "- %s — %s\n", n, t.byName[n])
		}
	}
	if len(already) > 0 {
		fmt.Fprintf(&b, "Already active: %s\n", strings.Join(already, ", "))
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
		return "", fmt.Errorf("invalid deactivate_tools input: %w", err)
	}
	removed := t.active.Deactivate(in.Names...)
	if len(removed) == 0 {
		return "No active tools matched; nothing deactivated.", nil
	}
	return "Deactivated: " + strings.Join(removed, ", "), nil
}

// ---- find_tools -----------------------------------------------------------

// FindToolsTool searches the load-on-demand catalog by keyword, so the model can
// discover the right tool name in MCP-heavy workspaces with large catalogs.
type FindToolsTool struct {
	entries []lazyEntry
}

// NewFindToolsTool builds the tool over a snapshot of the lazy catalog.
func NewFindToolsTool(catalog []providers.ToolDef) FindToolsTool {
	return FindToolsTool{entries: toLazyEntries(catalog)}
}

func (FindToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "find_tools",
		Description: "Search the on-demand tool catalog by keyword to find a tool's exact name before " +
			"activating it. Returns matching name — description lines. Useful when many tools are available.",
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

func (t FindToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid find_tools input: %w", err)
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
