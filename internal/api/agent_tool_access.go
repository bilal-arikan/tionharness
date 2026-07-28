package api

import (
	"net/http"
	"sort"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// toolAccessEntry is one tool row of the composer's read-only tool inspector:
// what it is, where it comes from, and which visibility tier it sits at. No
// schema is included — the popup is an at-a-glance list, not the tools screen.
type toolAccessEntry struct {
	Name        string `json:"name"`
	Label       string `json:"label"`  // un-namespaced name for MCP tools
	Description string `json:"description"`
	Source      string `json:"source"` // "builtin" | "mcp"
	Server      string `json:"server"` // MCP server display name (empty for built-ins)
	Category    string `json:"category,omitempty"`
	Visibility  string `json:"visibility"` // "full" | "summary" | "name-only" | "hidden"
}

// toolAccessServer is one MCP server as the inspector shows it: its config
// identity plus how many of its tools are eager/lazy for THIS agent and how many
// live pool connections it currently holds. A disabled server contributes no
// tools — it is listed so the user can see what could be turned on.
type toolAccessServer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Transport  string `json:"transport"`
	Scope      string `json:"scope"`
	Enabled    bool   `json:"enabled"`
	EagerCount int    `json:"eagerCount"`
	LazyCount  int    `json:"lazyCount"`
	Live       int    `json:"live"`  // alive pool connections right now
	Total      int    `json:"total"` // pool entries (alive or reconnecting)
}

// agentToolAccessResp is the whole payload: the effective per-turn tool split
// (eager schemas shipped every turn vs lazy tools the agent may activate through
// the gateway), the agent's blocked names, and the MCP server inventory.
type agentToolAccessResp struct {
	AgentID     string             `json:"agentId"`
	AgentName   string             `json:"agentName"`
	Provider    string             `json:"provider"`
	MCPEnabled  bool               `json:"mcpEnabled"`
	Eager       []toolAccessEntry  `json:"eager"`
	Lazy        []toolAccessEntry  `json:"lazy"`
	Blocked     []string           `json:"blocked"`
	Servers     []toolAccessServer `json:"servers"`
	PoolIdleSec int                `json:"poolIdleSec"`
}

// handleAgentToolAccess reports, read-only, which tools the agent can actually
// use right now: the eager set (schemas shipped every turn) and the lazy set
// (advertised in the load-on-demand catalog and activated via tool_search /
// activate_tools), plus the MCP servers behind them. It changes nothing — the
// composer's tool inspector renders it as pure information.
func (s *Server) handleAgentToolAccess(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	ag, err := wsp.DB.GetAgent(ctx, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Namespace prefix → display name, so MCP tools group under the server the
	// user configured rather than the sanitized prefix.
	serverByNS := map[string]string{}
	servers, err := wsp.DB.ListMCPServers(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, m := range servers {
		if ns, _, ok := mcp.SplitNamespaced(mcp.NamespaceTool(m.Name, "x")); ok {
			serverByNS[ns] = m.Name
		}
	}

	visibilityOf := wsp.Runtime.ToolVisibilityFunc(ctx, ag)
	eagerByServer := map[string]int{}
	lazyByServer := map[string]int{}

	// entriesOf renders a catalog slice into rows, counting MCP tools per server
	// into the supplied tally.
	entriesOf := func(defs []providers.ToolDef, tally map[string]int) []toolAccessEntry {
		out := make([]toolAccessEntry, 0, len(defs))
		for _, d := range defs {
			e := toolAccessEntry{
				Name:        d.Name,
				Label:       d.Name,
				Description: d.Description,
				Source:      "builtin",
				Category:    tools.CategoryOf(d.Name),
				Visibility:  visibilityOf(d.Name),
			}
			if ns, tool, ok := mcp.SplitNamespaced(d.Name); ok {
				e.Source = "mcp"
				e.Label = tool
				e.Category = ""
				if name, found := serverByNS[ns]; found {
					e.Server = name
				} else {
					e.Server = ns
				}
				tally[e.Server]++
			}
			out = append(out, e)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}

	eager := entriesOf(wsp.Runtime.ShippedToolCatalog(ctx, ag), eagerByServer)
	lazy := entriesOf(wsp.Runtime.LazyToolCatalog(ctx, ag), lazyByServer)

	// Live pool state per server name (a disabled or never-dialled server simply
	// has no entry and reports zero).
	live := map[string]mcpPoolServerAgg{}
	poolIdleSec := 0
	if pool := wsp.Runtime.MCPPool(); pool != nil {
		poolIdleSec = int(pool.IdleTTL().Seconds())
		for _, st := range pool.Stats() {
			agg := live[st.Server]
			agg.Server = st.Server
			agg.Total++
			if st.Alive {
				agg.Live++
			}
			live[st.Server] = agg
		}
	}

	serverRows := make([]toolAccessServer, 0, len(servers))
	for _, m := range servers {
		agg := live[m.Name]
		serverRows = append(serverRows, toolAccessServer{
			ID:         m.ID,
			Name:       m.Name,
			Transport:  m.Transport,
			Scope:      m.Scope,
			Enabled:    m.Enabled,
			EagerCount: eagerByServer[m.Name],
			LazyCount:  lazyByServer[m.Name],
			Live:       agg.Live,
			Total:      agg.Total,
		})
	}

	blocked := []string{}
	for name, tier := range agent.ParseToolOverrides(ag) {
		if tier == agent.TierBlocked {
			blocked = append(blocked, name)
		}
	}
	sort.Strings(blocked)

	writeJSON(w, http.StatusOK, agentToolAccessResp{
		AgentID:     ag.ID,
		AgentName:   ag.Name,
		Provider:    ag.Provider,
		MCPEnabled:  ag.MCPEnabled,
		Eager:       eager,
		Lazy:        lazy,
		Blocked:     blocked,
		Servers:     serverRows,
		PoolIdleSec: poolIdleSec,
	})
}
