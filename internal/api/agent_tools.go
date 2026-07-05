package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
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

// handleAgentTools returns the agent's tool access settings plus the catalog of
// tools it may pick from — the workspace-ACTIVE tools (built-ins + enabled MCP
// servers, minus the workspace denylist). The agent reaches every tool in this
// catalog by default; blockedTools is the per-agent denylist of tools switched
// off for this agent only (empty = all tools available).
func (s *Server) handleAgentTools(w http.ResponseWriter, r *http.Request) {
	agent, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	catalog := ws(r).Runtime.ActiveToolCatalog(r.Context())
	if catalog == nil {
		catalog = []providers.ToolDef{}
	}
	var blocked []string
	_ = json.Unmarshal([]byte(agent.BlockedTools), &blocked)
	if blocked == nil {
		blocked = []string{}
	}
	// Legacy migration: an agent configured under the old allowlist model still
	// has its restriction enforced at runtime. Translate that allowlist into the
	// equivalent denylist (everything in the catalog NOT allowed) so the new UI
	// shows the true effective set; the next save clears the allowlist for good.
	var allowed []string
	_ = json.Unmarshal([]byte(agent.AllowedTools), &allowed)
	if len(allowed) > 0 {
		blockedSet := map[string]bool{}
		for _, n := range blocked {
			blockedSet[n] = true
		}
		for _, t := range catalog {
			if !matchesPattern(t.Name, allowed) {
				blockedSet[t.Name] = true
			}
		}
		blocked = blocked[:0]
		for _, t := range catalog {
			if blockedSet[t.Name] {
				blocked = append(blocked, t.Name)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mcpEnabled":   agent.MCPEnabled,
		"blockedTools": blocked,
		"catalog":      catalog,
	})
}

type setAgentToolsReq struct {
	MCPEnabled   bool     `json:"mcpEnabled"`
	BlockedTools []string `json:"blockedTools"`
}

// handleSetAgentTools updates the agent's tool access settings (master switch +
// per-agent denylist).
func (s *Server) handleSetAgentTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setAgentToolsReq](w, r)
	if !ok {
		return
	}
	blockedJSON, _ := json.Marshal(req.BlockedTools)
	if err := ws(r).DB.UpdateAgentTools(r.Context(), id, req.MCPEnabled, string(blockedJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mcpEnabled": req.MCPEnabled, "blockedTools": req.BlockedTools})
}
