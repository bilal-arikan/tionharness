package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// spawnSessionReq is the body for POST /api/sessions/spawn.
type spawnSessionReq struct {
	AgentID       string `json:"agentId"`
	Prompt        string `json:"prompt"`
	ModelOverride string `json:"modelOverride"`
}

// handleSpawnSession opens a new, independent "spawned" session and runs the
// agent's turn in the background (fire-and-forget), returning the new session id
// immediately. The run surfaces live in the unified executions feed. This is the
// UI "Yeni oturum başlat" surface; the spawn_session agent tool shares the same
// Runtime.SpawnSession core.
func (s *Server) handleSpawnSession(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[spawnSessionReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	res, err := ws(r).Runtime.SpawnSession(r.Context(), req.AgentID, req.Prompt, agent.SpawnOptions{
		ModelOverride: req.ModelOverride,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("session spawn accepted", "session", res.SessionID, "agent", req.AgentID, "queued", res.Queued)
	// Cross-window sync: Runtime.SpawnSession itself emits the "session" event
	// (so the agent-initiated spawn_session tool path gets the same refresh), so
	// the handler does NOT publish a duplicate here.
	writeJSON(w, http.StatusCreated, struct {
		SessionID     string `json:"sessionId"`
		AgentName     string `json:"agentName"`
		Queued        bool   `json:"queued"`
		QueuePosition int    `json:"queuePosition,omitempty"`
	}{res.SessionID, res.AgentName, res.Queued, res.QueuePosition})
}
