package view

import (
	"fmt"
	"strings"
	"time"
)

// projectToolsSub renders one slice of the tool surface: a built-in tool group
// (Sub "group:<key>") or one MCP server (Sub "mcp:<id>"). These are the leaves
// under the Araçlar node of the Explorer map. An unknown selector is an error —
// a blank card would read like an empty but real group.
func projectToolsSub(in ToolsInput, level Level) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	ref := Ref{Kind: KindTools, ID: ToolsRefID, Sub: in.Sub}

	switch {
	case strings.HasPrefix(in.Sub, ToolsSubGroupPrefix):
		key := strings.TrimPrefix(in.Sub, ToolsSubGroupPrefix)
		for _, g := range in.Groups {
			if g.Key != key {
				continue
			}
			return projectToolGroup(ref, g, in, level, now), nil
		}
		return View{}, fmt.Errorf("view: tools: unknown group %q", key)
	case strings.HasPrefix(in.Sub, ToolsSubMCPPrefix):
		id := strings.TrimPrefix(in.Sub, ToolsSubMCPPrefix)
		for _, m := range in.MCPServers {
			if m.ID != id {
				continue
			}
			state := "○ kapalı"
			if m.Enabled {
				state = "● aktif"
			}
			v := View{
				Ref:    ref,
				Level:  level,
				AsOf:   now,
				Source: fmt.Sprintf("%s/%t", m.ID, m.Enabled),
			}
			v.Header = fmt.Sprintf("TOOLS · MCP %s · %s · asOf %s", clip(mcpName(m), 30), state, hhmmss(now))
			if level != LevelTiny {
				var l lines
				l.add("taşıma: %s", orDash(string(m.Transport)))
				l.add("hedef: %s", mcpEndpoint(m))
				v.Body = l.String()
			}
			v.finalize()
			return v, nil
		}
		return View{}, fmt.Errorf("view: tools: unknown MCP server %q", id)
	default:
		return View{}, fmt.Errorf("view: tools: unknown selector %q", in.Sub)
	}
}

// projectToolGroup lists a group's tool names. Workspace-disabled tools in the
// group are marked so the card says what the agent can actually call.
func projectToolGroup(ref Ref, g ToolGroup, in ToolsInput, level Level, now time.Time) View {
	disabled := map[string]bool{}
	for _, name := range in.ToolConfig.DisabledTools {
		disabled[name] = true
	}
	off := 0
	for _, name := range g.Tools {
		if disabled[name] {
			off++
		}
	}
	v := View{
		Ref:    ref,
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%s/%d/%d", g.Key, len(g.Tools), off),
	}
	v.Header = fmt.Sprintf("TOOLS · grup %s · %d araç (%d kapalı) · asOf %s",
		toolGroupLabel(g), len(g.Tools), off, hhmmss(now))
	if level == LevelTiny {
		v.finalize()
		return v
	}
	names := make([]string, 0, len(g.Tools))
	for _, name := range g.Tools {
		if disabled[name] {
			names = append(names, name+" (kapalı)")
		} else {
			names = append(names, name)
		}
	}
	var l lines
	if len(names) == 0 {
		l.add("(grupta araç yok)")
	} else {
		limit := len(names)
		if level != LevelFull && limit > toolsServerRows {
			limit = toolsServerRows
		}
		l.add("%s", strings.Join(names[:limit], ", "))
		if dropped := len(names) - limit; dropped > 0 {
			v.Elided, v.ElidedUnit = dropped, "araç"
		}
	}
	v.Body = l.String()
	v.finalize()
	return v
}
