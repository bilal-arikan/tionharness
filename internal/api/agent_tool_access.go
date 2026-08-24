package api

import (
	"net/http"
	"sort"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// toolAccessEntry is one tool row of the composer's read-only tool inspector:
// what it is, where it comes from, and which visibility tier it sits at. No
// schema is included — the popup is an at-a-glance list, not the tools screen.
type toolAccessEntry struct {
	Name        string `json:"name"`
	Label       string `json:"label"` // un-namespaced name for MCP tools
	Description string `json:"description"`
	Source      string `json:"source"` // "builtin" | "mcp"
	Server      string `json:"server"` // MCP server display name (empty for built-ins)
	Category    string `json:"category,omitempty"`
	Visibility  string `json:"visibility"` // "full" | "summary" | "name-only" | "hidden"
	// InContext reports whether the tool occupies prompt context right now: eager
	// tools always do (full schema), lazy tools do unless they are "hidden" — a
	// hidden tool is absent from the load-on-demand catalog block and only
	// reachable through tool_search, so it costs nothing until discovered.
	InContext bool `json:"inContext"`
}

// Context status of one MCP server, from the agent's point of view. This answers
// the only question that matters at a glance: are this server's tools actually in
// the prompt right now, and if not, why not?
const (
	serverStatusInContext  = "in-context"    // at least one tool sits in the prompt
	serverStatusHiddenOnly = "hidden-only"   // tools exist but none are catalogued (tool_search only)
	serverStatusDisabled   = "disabled"      // server switched off in the workspace
	serverStatusAgentOff   = "agent-mcp-off" // agent's master MCP switch is off
	serverStatusNoTools    = "no-tools"      // enabled but contributes nothing (not connected / all blocked)
)

// toolAccessServer is one MCP server as the inspector shows it: its config
// identity, whether it reaches the agent's context and how, and how many live
// pool connections it holds. A disabled server contributes no tools — it is
// listed so the user can see what could be turned on.
type toolAccessServer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Scope     string `json:"scope"`
	Enabled   bool   `json:"enabled"`
	// Status is one of the serverStatus* constants — the "is it in context" verdict.
	Status string `json:"status"`
	// EagerCount: full schemas shipped every turn. LazyCount: listed in the
	// load-on-demand catalog block (name/summary only). HiddenCount: reachable
	// solely via tool_search — NOT in the prompt. ContextCount = eager + lazy.
	EagerCount   int `json:"eagerCount"`
	LazyCount    int `json:"lazyCount"`
	HiddenCount  int `json:"hiddenCount"`
	ContextCount int `json:"contextCount"`
	Live         int `json:"live"`  // alive pool connections right now
	Total        int `json:"total"` // pool entries (alive or reconnecting)
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
	lazyByServer := map[string]int{}   // catalogued lazy tools (in the prompt)
	hiddenByServer := map[string]int{} // hidden lazy tools (tool_search only)

	// entriesOf renders a catalog slice into rows. eager marks the shipped set,
	// whose schemas are always in context; a lazy tool is in context only while it
	// is catalogued (not "hidden"). MCP tools are tallied per server accordingly.
	entriesOf := func(defs []providers.ToolDef, eager bool) []toolAccessEntry {
		out := make([]toolAccessEntry, 0, len(defs))
		for _, d := range defs {
			vis := visibilityOf(d.Name)
			e := toolAccessEntry{
				Name:        d.Name,
				Label:       d.Name,
				Description: d.Description,
				Source:      "builtin",
				Category:    tools.CategoryOf(d.Name),
				Visibility:  vis,
				InContext:   eager || vis != tools.VisibilityHidden,
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
				switch {
				case eager:
					eagerByServer[e.Server]++
				case e.InContext:
					lazyByServer[e.Server]++
				default:
					hiddenByServer[e.Server]++
				}
			}
			out = append(out, e)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}

	eager := entriesOf(wsp.Runtime.ShippedToolCatalog(ctx, ag), true)
	lazy := entriesOf(wsp.Runtime.LazyToolCatalog(ctx, ag), false)

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
		row := toolAccessServer{
			ID:           m.ID,
			Name:         m.Name,
			Transport:    m.Transport,
			Scope:        m.Scope,
			Enabled:      m.Enabled,
			EagerCount:   eagerByServer[m.Name],
			LazyCount:    lazyByServer[m.Name],
			HiddenCount:  hiddenByServer[m.Name],
			ContextCount: eagerByServer[m.Name] + lazyByServer[m.Name],
			Live:         agg.Live,
			Total:        agg.Total,
		}
		// Verdict order matters: report the OUTERMOST reason a server is absent
		// from context first (workspace switch, then agent switch), so the user
		// fixes the right knob instead of chasing an empty tool list.
		switch {
		case !m.Enabled:
			row.Status = serverStatusDisabled
		case !ag.MCPEnabled:
			row.Status = serverStatusAgentOff
		case row.ContextCount > 0:
			row.Status = serverStatusInContext
		case row.HiddenCount > 0:
			row.Status = serverStatusHiddenOnly
		default:
			row.Status = serverStatusNoTools
		}
		serverRows = append(serverRows, row)
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
