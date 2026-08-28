package tools

import (
	"fmt"
	"sort"
	"strings"
)

// ToolSearchMaxRows is how many tool_search hits are rendered before the
// overflow notice takes over, on both the native and the gateway path.
const ToolSearchMaxRows = 30

// ToolSearchRow is one already-ranked tool_search hit, in render order.
type ToolSearchRow struct {
	Name string
	Desc string
}

// ToolSearchRenderOpts holds the parts of the tool_search result rendering that
// legitimately differ between the two call paths — the native builtin
// (ToolSearchTool.Call) and the claude-cli gateway (interactionBackend.callToolSearch).
// Everything NOT expressed here is shared and must stay byte-identical on both paths;
// the options exist so that fixing the shared part once fixes it everywhere.
type ToolSearchRenderOpts struct {
	// Header is the first line, printed verbatim. The two paths word the
	// activation hint differently ("activate with" vs "load with").
	Header string
	// Max is how many rows survive before the overflow notice replaces the rest.
	Max int
	// DescLimit truncates a description to that many BYTES plus an ellipsis.
	// Zero means no truncation (the native path prints descriptions in full).
	DescLimit int
	// BundleOf resolves a tool name to its bundle key for the trailing "[key]"
	// tag. Returning "" leaves the row untagged — the gateway does that for MCP
	// bundles, which it never serves. Required.
	BundleOf func(string) string
	// OmitEmptyDroppedBundles drops the "The rest live in: …" clause when the
	// overflowed rows resolved to no bundle key at all. The native path always
	// emits the clause; the gateway omits it.
	OmitEmptyDroppedBundles bool
}

// RenderToolSearch renders ranked tool_search hits into the model-facing result
// text: a header, one "- name — desc  [bundle]" row per hit, and an overflow
// notice naming the bundles the dropped hits live in so narrowing has a
// direction. Pure: it reads nothing but its arguments.
func RenderToolSearch(rows []ToolSearchRow, opts ToolSearchRenderOpts) string {
	if opts.BundleOf == nil {
		panic("tools: RenderToolSearch requires a BundleOf resolver")
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		desc := r.Desc
		if opts.DescLimit > 0 && len(desc) > opts.DescLimit {
			desc = desc[:opts.DescLimit] + "…"
		}
		line := "- " + r.Name + " — " + desc
		if key := opts.BundleOf(r.Name); key != "" {
			line += "  [" + key + "]"
		}
		lines = append(lines, line)
	}
	more := ""
	if opts.Max > 0 && len(lines) > opts.Max {
		hidden := len(lines) - opts.Max
		var dropped []string
		seen := map[string]bool{}
		for _, r := range rows[opts.Max:] {
			key := opts.BundleOf(r.Name)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			dropped = append(dropped, key)
		}
		sort.Strings(dropped)
		more = fmt.Sprintf("\n…and %d more; refine the query.", hidden)
		if len(dropped) > 0 || !opts.OmitEmptyDroppedBundles {
			more += " The rest live in: " + strings.Join(dropped, ", ") + "."
		}
		lines = lines[:opts.Max]
	}
	return opts.Header + "\n" + strings.Join(lines, "\n") + more
}
