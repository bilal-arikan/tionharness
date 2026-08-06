package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// ToolsInput is everything the tools projection reads: the configured MCP server
// pool and the workspace-level tool activation document. Both are in-memory store
// reads.
type ToolsInput struct {
	MCPServers []db.MCPServer
	ToolConfig db.WorkspaceToolConfig
	// Now is the clock used for the asOf stamp. Zero means time.Now().
	Now time.Time
}

// toolsServerRows is how many MCP servers a card-level tools view names before the
// rest are reported as Elided. LevelFull lists them all.
const toolsServerRows = 12

// ProjectTools renders the workspace tool surface: how many MCP servers are
// configured and enabled, and how many tools are switched off workspace-wide.
func ProjectTools(in ToolsInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	enabled := 0
	for _, m := range in.MCPServers {
		if m.Enabled {
			enabled++
		}
	}
	disabledTools := len(in.ToolConfig.DisabledTools)

	v := View{
		Ref:    Ref{Kind: KindTools, ID: ToolsRefID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%d/%d", len(in.MCPServers), enabled, disabledTools),
	}
	v.Header = fmt.Sprintf("TOOLS · %d MCP sunucu (%d aktif) · %d araç kapalı · asOf %s",
		len(in.MCPServers), enabled, disabledTools, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	servers := append([]db.MCPServer(nil), in.MCPServers...)
	// Enabled servers first, then newest — the reader cares about the live pool
	// before the dormant configs.
	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].Enabled != servers[j].Enabled {
			return servers[i].Enabled
		}
		return servers[i].CreatedAt > servers[j].CreatedAt
	})

	limit := len(servers)
	if level != LevelFull && limit > toolsServerRows {
		limit = toolsServerRows
	}
	for _, m := range servers[:limit] {
		state := "○ kapalı"
		if m.Enabled {
			state = "● aktif"
		}
		l.add("%s %-20s %s", state, clip(mcpName(m), 20), mcpEndpoint(m))
	}
	if dropped := len(servers) - limit; dropped > 0 {
		v.Elided, v.ElidedUnit = dropped, "MCP sunucu"
	}

	if disabledTools > 0 && lens != LensErrors {
		l.add("kapalı araçlar: %s", strings.Join(clipList(in.ToolConfig.DisabledTools, toolsServerRows), ", "))
	}
	if l.empty() {
		l.add("(yapılandırılmış MCP sunucusu yok)")
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}

// mcpName falls back to the id when a server has no name.
func mcpName(m db.MCPServer) string {
	if m.Name == "" {
		return m.ID
	}
	return m.Name
}

// mcpEndpoint renders a server's connection target — the command for a stdio
// subprocess, the URL for an HTTP server.
func mcpEndpoint(m db.MCPServer) string {
	if m.Transport == db.MCPTransportHTTP || m.Transport == db.MCPTransportSSE {
		return clip(orDash(m.URL), 50)
	}
	return clip(orDash(m.Command), 50)
}
