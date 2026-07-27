package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
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
	writeJSON(w, http.StatusOK, map[string]any{
		"mcpEnabled":    ag.MCPEnabled,
		"toolOverrides": overrides,
		"blockedTools":  blocked, // derived; kept for older clients
		"catalog":       out,
	})
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
