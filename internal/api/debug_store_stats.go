package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// Store footprint diagnostics.
//
// Every workspace loads its entire message history into RAM at boot, and with a
// handful of long-lived workspaces that is the process's single largest
// allocation. This endpoint reports it per workspace so the lazy-loading work
// (Madde 5) can be measured rather than assumed — and so a user reporting "it
// eats memory" can be answered with the actual split.

// workspaceStoreStats is one workspace's footprint line. StoreStats is embedded
// untagged so its fields flatten into the same JSON object as the workspace
// identity, keeping each row one flat record.
type workspaceStoreStats struct {
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	db.StoreStats
}

type storeStatsResp struct {
	Workspaces []workspaceStoreStats `json:"workspaces"`
	// Totals sums the per-workspace rows so the headline number needs no
	// client-side arithmetic.
	Totals db.StoreStats `json:"totals"`
}

// handleStoreStats reports the in-memory store footprint of every open
// workspace. GET /api/debug/store-stats
//
// A workspace that fails to resolve is SKIPPED rather than failing the whole
// request — a diagnostic that refuses to answer because one workspace is broken
// is useless exactly when it is needed. The gap is visible: its row is absent
// from a response the caller can compare against the workspace list.
func (s *Server) handleStoreStats(w http.ResponseWriter, r *http.Request) {
	out := storeStatsResp{Workspaces: []workspaceStoreStats{}}
	if s.workspaces == nil {
		// No workspace manager wired (bare/test server). An empty report is the
		// truthful answer — the process holds no stores — and mirrors how
		// workspaceByID refuses to dereference a nil manager.
		writeJSON(w, http.StatusOK, out)
		return
	}
	for _, meta := range s.workspaces.List() {
		wsp, err := s.workspaces.Get(meta.ID)
		if err != nil || wsp == nil || wsp.DB == nil {
			continue
		}
		st := wsp.DB.Stats()
		out.Workspaces = append(out.Workspaces, workspaceStoreStats{
			WorkspaceID:   meta.ID,
			WorkspaceName: meta.Name,
			StoreStats:    st,
		})
		out.Totals.Sessions += st.Sessions
		out.Totals.LoadedSessions += st.LoadedSessions
		out.Totals.Messages += st.Messages
		out.Totals.MessageBytes += st.MessageBytes
		out.Totals.Agents += st.Agents
		out.Totals.Tasks += st.Tasks
		out.Totals.Artifacts += st.Artifacts
		out.Totals.Flows += st.Flows
		out.Totals.Automations += st.Automations
	}
	writeJSON(w, http.StatusOK, out)
}
