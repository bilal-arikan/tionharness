package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// matchesPattern reports whether name matches any pattern (suffix "*" = prefix
// match, else exact). Mirrors the runtime tool predicate.
func matchesPattern(name string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if name == p {
			return true
		}
	}
	return false
}

// agentToolEntry is one row of the agent tools screen: a workspace-ACTIVE tool
// plus the visibility tier it would get WITHOUT any per-agent override
// (defaultVisibility). The UI compares that baseline against the agent's
// override map and renders only the differences.
type agentToolEntry struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	DefaultVisibility string          `json:"defaultVisibility"` // "full" | "summary" | "name-only" | "hidden"
	InputSchema       json.RawMessage `json:"inputSchema,omitempty"`
}

// handleAgentTools returns the agent's tool access settings plus the catalog of
// tools it may pick from — the workspace-ACTIVE tools (built-ins + enabled MCP
// servers, minus the workspace denylist), each tagged with its workspace-
// effective (i.e. pre-override) visibility tier.
//
// toolOverrides is the per-agent override map (tool name or "prefix*" pattern →
// one of the four visibility tiers or "blocked"); an absent key means the tool
// follows its defaultVisibility. blockedTools is returned alongside for
// backwards compatibility — it is the derived "blocked" slice of the same map.
func (s *Server) handleAgentTools(w http.ResponseWriter, r *http.Request) {
	ag, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	catalog, defaultVis := ws(r).Runtime.ActiveToolCatalogWithState(r.Context())
	out := make([]agentToolEntry, 0, len(catalog))
	names := make([]string, 0, len(catalog))
	for _, t := range catalog {
		out = append(out, agentToolEntry{
			Name:              t.Name,
			Description:       t.Description,
			DefaultVisibility: defaultVis[t.Name],
			InputSchema:       t.InputSchema,
		})
		names = append(names, t.Name)
	}

	overrides := agent.ParseToolOverrides(ag)
	// Legacy migration: an agent configured under the old allowlist model still has
	// its restriction enforced at runtime. Translate that allowlist into equivalent
	// "blocked" overrides (everything in the catalog NOT allowed) so the UI shows
	// the true effective set; the next save clears the allowlist for good.
	var allowed []string
	_ = json.Unmarshal([]byte(ag.AllowedTools), &allowed)
	if len(allowed) > 0 {
		for _, name := range names {
			if !matchesPattern(name, allowed) {
				overrides[name] = agent.TierBlocked
			}
		}
	}

	blocked := []string{}
	for name, tier := range overrides {
		if tier == agent.TierBlocked {
			blocked = append(blocked, name)
		}
	}
	groups, err := agentToolGroups(r.Context(), ws(r), names)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mcpEnabled":    ag.MCPEnabled,
		"toolOverrides": overrides,
		"blockedTools":  blocked, // derived; kept for older clients
		"catalog":       out,
		"groups":        groups,
	})
}

// agentToolGroup is one bulk-override target: a functional category of built-in
// tools ("group:files") or an MCP server's namespace pattern ("linear__*").
// Both are ordinary override keys — the group block in the UI is just a faster
// way to write one key instead of ten.
type agentToolGroup struct {
	Key   string   `json:"key"`
	Kind  string   `json:"kind"` // "builtin" | "mcp"
	Label string   `json:"label"`
	Count int      `json:"count"`
	Tools []string `json:"tools"`
}

// agentToolGroups derives the group rows from the ACTIVE catalog: only groups
// that actually have tools right now are returned, so the UI never offers a key
// that would match nothing. Built-in categories come first in their canonical
// order, then MCP servers sorted by namespace.
func agentToolGroups(ctx context.Context, wsp *workspace.Workspace, names []string) ([]agentToolGroup, error) {
	byCategory := map[string][]string{}
	byNS := map[string][]string{}
	for _, name := range names {
		if ns, _, ok := mcp.SplitNamespaced(name); ok {
			byNS[ns] = append(byNS[ns], name)
			continue
		}
		cat := tools.CategoryOf(name)
		byCategory[cat] = append(byCategory[cat], name)
	}

	out := make([]agentToolGroup, 0, len(byCategory)+len(byNS))
	for _, cat := range tools.Categories() {
		members := byCategory[cat]
		if len(members) == 0 {
			continue
		}
		sort.Strings(members)
		out = append(out, agentToolGroup{
			Key:   tools.GroupPrefix + cat,
			Kind:  "builtin",
			Label: cat,
			Count: len(members),
			Tools: members,
		})
	}

	// Namespace prefix → configured display name, so an MCP group reads as the
	// server the user named rather than its sanitized prefix.
	serverByNS := map[string]string{}
	servers, err := wsp.DB.ListMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range servers {
		if ns, _, ok := mcp.SplitNamespaced(mcp.NamespaceTool(m.Name, "x")); ok {
			serverByNS[ns] = m.Name
		}
	}
	nss := make([]string, 0, len(byNS))
	for ns := range byNS {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	for _, ns := range nss {
		members := byNS[ns]
		sort.Strings(members)
		label := ns
		if name, found := serverByNS[ns]; found {
			label = name
		}
		out = append(out, agentToolGroup{
			Key:   ns + "__*",
			Kind:  "mcp",
			Label: label,
			Count: len(members),
			Tools: members,
		})
	}
	return out, nil
}

// setAgentToolsReq carries the agent's master tool switch plus its override map.
// BlockedTools is the LEGACY field: a client that only knows the old denylist
// model may still send it, and it is folded in as "blocked" overrides.
type setAgentToolsReq struct {
	MCPEnabled    bool              `json:"mcpEnabled"`
	ToolOverrides map[string]string `json:"toolOverrides"`
	BlockedTools  []string          `json:"blockedTools"` // legacy
}

// handleSetAgentTools updates the agent's tool access settings: the master
// switch plus the per-agent override map. An invalid tier is rejected so a typo
// cannot silently leave a tool at its default.
func (s *Server) handleSetAgentTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setAgentToolsReq](w, r)
	if !ok {
		return
	}
	overrides := map[string]string{}
	for name, tier := range req.ToolOverrides {
		// A "group:" key must name a KNOWN category. Dropping an unknown one
		// silently would leave the user believing a ban is in force when nothing
		// matches it, so it is a hard 400.
		if strings.HasPrefix(name, tools.GroupPrefix) && !tools.ValidGroupKey(name) {
			writeError(w, http.StatusBadRequest, "unknown tool group: "+name)
			return
		}
		if !agent.ValidAgentTier(tier) {
			writeError(w, http.StatusBadRequest, "invalid tier for "+name+": "+tier)
			return
		}
		overrides[name] = tier
	}
	// A legacy blockedTools payload contributes "blocked" entries. An explicit
	// override for the same name wins (the newer field is the intent).
	for _, name := range req.BlockedTools {
		if _, explicit := overrides[name]; !explicit {
			overrides[name] = agent.TierBlocked
		}
	}
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ws(r).DB.UpdateAgentTools(r.Context(), id, req.MCPEnabled, string(overridesJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mcpEnabled": req.MCPEnabled, "toolOverrides": overrides})
}
