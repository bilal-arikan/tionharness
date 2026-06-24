package api

// handleSessionUsageDetail returns a single session's lifetime spend and savings:
// total calls/tokens, a per-origin (ByKind) and per-model breakdown with cost
// (reusing the same Motor-B pricing helpers as the workspace Budget screen), the
// prompt-cache USD savings, and the tool-output compaction byte meters (System A
// + System B). This is the data behind the per-session budget card in the
// session detail panel — the session-scoped analog of /api/usage.

import (
	"net/http"
)

func (s *Server) handleSessionUsageDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	wsp := ws(r)

	u, err := wsp.DB.GetSessionUsage(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}

	// Cost + per-model detail come from the shared helper so per-session figures
	// match the workspace Budget screen and the agent usage endpoint exactly.
	models, cost, savings, priced, estimated, cacheRead, cacheWrite := modelRowsFor(u.ByModel)

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId":        u.SessionID,
		"agentId":          u.AgentID,
		"calls":            u.Calls,
		"inputTokens":      u.InputTokens,
		"outputTokens":     u.OutputTokens,
		"cacheReadTokens":  cacheRead,
		"cacheWriteTokens": cacheWrite,
		"byKind":           u.ByKind, // per-origin breakdown (chat/task/schedule/flow/compact/…)
		"byModel":          models,   // per-model detail with cost
		"costUSD":          cost,
		"savingsUSD":       savings, // prompt-cache reads vs full input price
		"priced":           priced,
		"estimated":        estimated,
		"compactSavedBytes":    u.CompactSavedBytes,    // System A: deterministic tool-output trim
		"compactSavedBytesLLM": u.CompactSavedBytesLLM, // System B: LLM summary trim
	})
}
