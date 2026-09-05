package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ToolGroup is one functional bucket of built-in tools (files, search, agents…)
// as the Explorer map shows it under the Araçlar node: the group name and the
// tool names in it, nothing deeper. The grouping itself lives in internal/tools
// (categories.go); view cannot import that package (tools imports view), so the
// projector receives it through a source.
type ToolGroup struct {
	Key   string
	Label string
	Tools []string
}

// ToolGroupsSource enumerates the built-in tool groups. Nil means the map shows
// only the MCP servers under Araçlar.
type ToolGroupsSource interface {
	ToolGroups() []ToolGroup
}

// Sub selectors of the tools and budget nodes. A ref with one of these subs is a
// leaf of the map that projects the named slice of its parent's surface.
const (
	ToolsSubGroupPrefix     = "group:"
	ToolsSubMCPPrefix       = "mcp:"
	BudgetSubProviderPrefix = "provider:"
)

// toolsChildren is the Araçlar node's structural children: one leaf per built-in
// tool group (when a source is attached) and one per configured MCP server, in
// the order the tools screen uses (groups by declared order, servers enabled
// first then newest).
func (p *Projector) toolsChildren(in ToolsInput) []Handle {
	hs := make([]Handle, 0, len(in.Groups)+len(in.MCPServers))
	for _, g := range in.Groups {
		hs = append(hs, Handle{
			Label: fmt.Sprintf("%s (%d araç)", toolGroupLabel(g), len(g.Tools)),
			Ref:   Ref{Kind: KindTools, ID: ToolsRefID, Sub: ToolsSubGroupPrefix + g.Key},
			Level: LevelCard,
		})
	}
	for _, m := range sortedMCPServers(in.MCPServers) {
		state := "kapalı"
		if m.Enabled {
			state = "aktif"
		}
		hs = append(hs, Handle{
			Label: fmt.Sprintf("MCP %s [%s]", clip(mcpName(m), 30), state),
			Ref:   Ref{Kind: KindTools, ID: ToolsRefID, Sub: ToolsSubMCPPrefix + m.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// budgetChildren is the Bütçe node's structural children: one leaf per provider
// that has priced spend on the covered day, costliest first. A provider with no
// usage today has no node — the map shows what is being spent, not the catalog.
func budgetChildren(in BudgetInput) []Handle {
	type acc struct {
		cost      float64
		models    int
		estimated bool
	}
	byProvider := map[string]*acc{}
	order := []string{}
	for _, row := range in.Rollup.Rows {
		name := row.Provider
		if name == "" {
			name = "-"
		}
		a, ok := byProvider[name]
		if !ok {
			a = &acc{}
			byProvider[name] = a
			order = append(order, name)
		}
		a.cost += row.CostUSD
		a.models++
		if row.Estimated || !row.Priced {
			a.estimated = true
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		if byProvider[order[i]].cost != byProvider[order[j]].cost {
			return byProvider[order[i]].cost > byProvider[order[j]].cost
		}
		return order[i] < order[j]
	})
	hs := make([]Handle, 0, len(order))
	for _, name := range order {
		a := byProvider[name]
		hs = append(hs, Handle{
			Label: fmt.Sprintf("%s · %s · %d model", name, usd(a.cost, a.estimated), a.models),
			Ref:   Ref{Kind: KindBudget, ID: BudgetRefID, Sub: BudgetSubProviderPrefix + name},
			Level: LevelCard,
		})
	}
	return hs
}

// toolGroupLabel prefers the source's display label and falls back to the key.
func toolGroupLabel(g ToolGroup) string {
	if strings.TrimSpace(g.Label) != "" {
		return g.Label
	}
	return g.Key
}

// sortedMCPServers orders the pool the way ProjectTools lists it: enabled first,
// then newest. Copies the input; the caller's slice may be a cache snapshot.
func sortedMCPServers(servers []db.MCPServer) []db.MCPServer {
	sorted := append([]db.MCPServer(nil), servers...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Enabled != sorted[j].Enabled {
			return sorted[i].Enabled
		}
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})
	return sorted
}
