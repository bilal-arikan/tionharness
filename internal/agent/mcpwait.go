package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// newMCPWaiter builds the wait_for_mcp_servers implementation for ONE registry
// build: it captures the agent whose turn this is and the registry that turn
// ships, so a warmed server's tools land in the caller's own live tool set.
//
// scopeKey is the SAME "<sessionID>|<agentID>" key buildRegistry computes for the
// catalog (see toolsetup.go): a scoped server must warm the caller's own pool
// slot, not a second connection nobody will dispatch to.
func (r *Runtime) newMCPWaiter(reg *tools.Registry, agent db.Agent, scopeKey string) tools.MCPWaiter {
	return func(ctx context.Context, want []string) ([]tools.MCPServerWaitResult, error) {
		return r.waitForMCPServers(ctx, reg, agent, scopeKey, want)
	}
}

// waitForMCPServers dials the requested servers in parallel, merges the tools of
// the ones that came up into the live registry, and reports a verdict per server.
//
// Circuit-breaker policy is deliberate and narrow: a server whose breaker is OPEN
// is dialed anyway, exactly once, by this tool. The breaker exists to stop a hung
// server from stalling every AUTOMATIC catalog build; an explicit "wait for this
// server" is the opposite situation — the caller has just fixed or restarted it
// and is knowingly paying the timeout. The outcome then updates the breaker
// normally: Clear on success (so the next ordinary build picks the server up
// without waiting out the cooldown), Note on failure (so the automatic path stays
// protected).
func (r *Runtime) waitForMCPServers(ctx context.Context, reg *tools.Registry, agent db.Agent, scopeKey string, want []string) ([]tools.MCPServerWaitResult, error) {
	if r.mcpPool == nil {
		return nil, fmt.Errorf("wait_for_mcp_servers: MCP is not available in this workspace")
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("wait_for_mcp_servers: listing enabled MCP servers: %w", err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("wait_for_mcp_servers: no MCP servers are enabled in this workspace")
	}

	selected, err := selectMCPServers(servers, want)
	if err != nil {
		return nil, err
	}

	cfgs := make([]mcp.ServerConfig, 0, len(selected))
	for _, m := range selected {
		cfg := toServerConfig(m)
		if m.Scope == "scoped" && scopeKey != "" {
			cfg.ScopeKey = scopeKey
		}
		// Same scratchpad-root treatment the catalog build applies, so a server
		// warmed here is configured identically to one dialed by the normal path —
		// otherwise the fingerprint would differ and the next build would re-dial.
		cfg = r.applyMCPScratchpadRoot(ctx, cfg, m, scopeKey)
		cfgs = append(cfgs, cfg)
	}

	outcomes := r.mcpPool.EnsureServers(ctx, cfgs)

	// Merge the live servers' tools into THIS turn's registry. The tool loop
	// recomputes the active tool set every iteration, so these become callable on
	// the model's next turn through the loop — the whole point of the tool.
	entries := mcp.CatalogEntries(outcomes)
	if len(entries) > 0 {
		cfgByServer := map[string]mcp.ServerConfig{}
		for _, cfg := range cfgs {
			name, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(cfg.Name, "x"))
			cfgByServer[name] = cfg
		}
		reg.AppendMCP(entries, cfgByServer)
		// Merging is not enough on its own. MCP tools are lazy+name-only, so
		// ActiveDefs would omit them until something activates them — and under a
		// FROZEN prompt epoch (mergeFrozenToolDefs) a non-active name is dropped from
		// the shipped block entirely. Activating them here is both the mechanism and
		// the correct semantics: the agent explicitly asked for these servers, so
		// their tools are what it intends to call next.
		if active := activeToolsFromCtx(ctx); active != nil {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.NamespacedName)
			}
			active.Activate(names...)
		}
	}

	toolCount := map[string]int{}
	for _, e := range entries {
		toolCount[e.Server]++
	}
	results := make([]tools.MCPServerWaitResult, 0, len(outcomes))
	for _, o := range outcomes {
		ready := o.State == mcp.ServerAlive
		if ready {
			// The server answered: close its breaker so the next ordinary catalog
			// build dials it immediately instead of waiting out the cooldown.
			r.mcpFailStreaks.Clear(o.Server)
		} else {
			streak := r.mcpFailStreaks.Note(o.Server)
			r.logger.Warn("wait_for_mcp_servers: server did not come up",
				"server", o.Server, "consecutive", streak, "error", o.Err)
		}
		results = append(results, tools.MCPServerWaitResult{
			Server: o.Server,
			Ready:  ready,
			Tools:  toolCount[o.Server],
			Err:    o.Err,
		})
	}
	return results, nil
}

// selectMCPServers resolves the requested names against the enabled servers. An
// empty request means all of them. An unknown name is an ERROR, not a silent
// skip: the model asked about a server that does not exist here, and answering
// "not ready" would send it debugging a connection instead of its typo.
func selectMCPServers(enabled []db.MCPServer, want []string) ([]db.MCPServer, error) {
	if len(want) == 0 {
		return enabled, nil
	}
	byName := make(map[string]db.MCPServer, len(enabled))
	for _, m := range enabled {
		byName[strings.ToLower(m.Name)] = m
	}
	var out []db.MCPServer
	var unknown []string
	seen := map[string]bool{}
	for _, name := range want {
		m, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		if seen[m.Name] {
			continue
		}
		seen[m.Name] = true
		out = append(out, m)
	}
	if len(unknown) > 0 {
		available := make([]string, 0, len(enabled))
		for _, m := range enabled {
			available = append(available, m.Name)
		}
		slices.Sort(available)
		return nil, fmt.Errorf("wait_for_mcp_servers: no enabled MCP server named %s (enabled: %s)",
			strings.Join(unknown, ", "), strings.Join(available, ", "))
	}
	return out, nil
}
