package api

import (
	"context"
	"net/http"
	"os/exec"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// handleAgentPath returns the absolute path of an agent's on-disk JSON file.
func (s *Server) handleAgentPath(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := ws(r).DB.AgentPath(id)
	if writeDBError(w, err, "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleRevealAgent opens the folder holding the agent's JSON file in the OS
// file manager (Windows: Explorer, highlighting the file) on the machine running
// the backend (local desktop app).
func (s *Server) handleRevealAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := ws(r).DB.AgentPath(id)
	if writeDBError(w, err, "agent not found") {
		return
	}
	// /select highlights the specific agent file inside the agents/ folder.
	// Detached from r.Context(): a fire-and-forget launch must not be killed when
	// the HTTP handler returns (CommandContext would race explorer to death).
	if err := exec.Command("explorer.exe", "/select,"+path).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// failure to *start* the process is a real error.
		s.logger.Warn("reveal agent folder failed", "agent", id, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleListAgents returns the workspace roster INCLUDING agents marked deleted
// (each flagged with deleted:true). The client needs them to render the author
// of a past conversation — the session outlives its agent — and filters them out
// of its own pickers. Server-side code paths that must never select a deleted
// agent use db.ListAgents, which excludes them.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := ws(r).DB.ListAgentsWithDeleted(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if agents == nil {
		agents = []db.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

type createAgentReq struct {
	Name           string `json:"name"`
	Soul           string `json:"soul"`
	Identity       string `json:"identity"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ThinkingLevel  string `json:"thinkingLevel"`
	PermissionMode string `json:"permissionMode"`
	Avatar         string `json:"avatar"`
	Color          string `json:"color"`
	// MCPEnabled gates tool access. Pointer so we can tell "omitted" (nil →
	// default on) apart from an explicit false (opt-out). New agents get tools
	// by default.
	MCPEnabled *bool `json:"mcpEnabled"`
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createAgentReq](w, r)
	if !ok {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Fall back for any blank field. Precedence: request value → the first
	// existing agent in this workspace (its concrete provider/model) → built-in
	// last resort. There is no abstract "default provider/model" any more (neither
	// per-workspace nor app-global): a new agent copies a real agent's setup, or —
	// when it is the very first agent — the keyless local claude-cli with the
	// provider's own default model.
	cfg := s.settings.Get()
	fp, fm := s.firstAgentProviderModel(r.Context(), ws(r))
	if req.Provider == "" {
		req.Provider = fp
	}
	if req.Provider == "" {
		req.Provider = "claude-cli" // last-resort: local Claude Code login, no API key
	}
	if req.Model == "" {
		req.Model = fm // may stay "" → provider applies its own default model
	}
	// Permission mode: request → application default → "auto" (db also defaults).
	if req.PermissionMode == "" {
		req.PermissionMode = cfg.DefaultPermissionMode
	}
	// Tool access defaults to ON for new agents; an explicit mcpEnabled:false in
	// the request opts out.
	mcpEnabled := true
	if req.MCPEnabled != nil {
		mcpEnabled = *req.MCPEnabled
	}

	agent, err := ws(r).DB.CreateAgent(r.Context(), db.Agent{
		Name:           req.Name,
		Soul:           req.Soul,
		Identity:       req.Identity,
		Provider:       req.Provider,
		Model:          req.Model,
		ThinkingLevel:  req.ThinkingLevel,
		PermissionMode: req.PermissionMode,
		MCPEnabled:     mcpEnabled,
	})
	if writeDBError(w, err, "") {
		return
	}

	if req.Avatar != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), agent.ID, db.AgentProfilePatch{Avatar: &req.Avatar})
		agent.Avatar = req.Avatar
	}
	if req.Color != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), agent.ID, db.AgentProfilePatch{Color: &req.Color})
		agent.Color = req.Color
	}

	s.logger.Info("agent created", "agent", agent.Name, "id", agent.ID,
		"provider", agent.Provider, "model", agent.Model)
	writeJSON(w, http.StatusCreated, agent)
}

// firstAgentProviderModel returns the provider/model of the first (newest)
// existing agent in the workspace, or empty strings when none exist. New agents
// inherit a real agent's concrete setup instead of an abstract workspace default.
func (s *Server) firstAgentProviderModel(ctx context.Context, wsp *workspace.Workspace) (provider, model string) {
	agents, err := wsp.DB.ListAgents(ctx)
	if err != nil || len(agents) == 0 {
		return "", ""
	}
	return agents[0].Provider, agents[0].Model
}

// agentRunning reports whether the agent has work in flight right now: a turn in
// any session it owns or takes part in (interactive OR autonomous), or a task run
// assigned to it. There is no per-agent run registry — both live registries are
// keyed by session — so this intersects the active session ids with the agent's
// own, the same way workspaceRunning unions them for the activity report.
// Returns the id of the first live thing found, for the error message.
func (s *Server) agentRunning(ctx context.Context, wsp *workspace.Workspace, agentID string) (bool, string) {
	live := s.runs.activeSessionIDs(wsp.ID)
	if wsp.Runtime != nil { // absent in tests and during early boot
		live = append(live, wsp.Runtime.ActiveSessionIDs()...)
	}
	for _, sid := range live {
		sess, err := wsp.DB.GetSession(ctx, sid)
		if err != nil {
			continue // vanished between the snapshot and the lookup
		}
		if sess.AgentID == agentID {
			return true, sess.ID
		}
		// A multi-agent thread: the agent may be answering as a participant even
		// though another agent owns the session.
		for _, p := range sess.Participants {
			if p == agentID {
				return true, sess.ID
			}
		}
	}
	if runs, err := wsp.DB.ListRunningRuns(ctx); err == nil {
		for _, run := range runs {
			if run.AgentID == agentID {
				return true, run.ID
			}
		}
	}
	return false, ""
}

// handleDeleteAgent marks the agent deleted and drops the schedules and tasks it
// owns. The sessions it owns are KEPT — see db.DeleteAgent. Refuses while the
// agent has a turn in flight: deleting mid-run would pull the roster entry out
// from under a turn that is still writing.
func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	if running, where := s.agentRunning(r.Context(), wsp, id); running {
		writeError(w, http.StatusConflict,
			"Ajan şu anda çalışıyor ("+where+"). Önce turu durdurun, sonra silin.")
		return
	}
	if err := wsp.DB.DeleteAgent(r.Context(), id); writeDBError(w, err, "agent not found") {
		return
	}
	// DeleteAgent also drops the agent's schedules; reload so their cron jobs
	// leave the live registry too.
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload after agent delete failed", "error", err)
	}
	s.logger.Info("agent deleted", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

type updateAgentReq struct {
	Name           *string   `json:"name"`
	Soul           *string   `json:"soul"`
	Identity       *string   `json:"identity"`
	Provider       *string   `json:"provider"`
	Model          *string   `json:"model"`
	ThinkingLevel  *string   `json:"thinkingLevel"`
	PermissionMode *string   `json:"permissionMode"`
	Avatar         *string   `json:"avatar"`
	Color          *string   `json:"color"`
	Skills         *[]string `json:"skills"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[updateAgentReq](w, r)
	if !ok {
		return
	}
	if req.Name != nil && *req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	agent, err := ws(r).DB.UpdateAgent(r.Context(), r.PathValue("id"), db.AgentProfilePatch{
		Name:           req.Name,
		Soul:           req.Soul,
		Identity:       req.Identity,
		Provider:       req.Provider,
		Model:          req.Model,
		ThinkingLevel:  req.ThinkingLevel,
		PermissionMode: req.PermissionMode,
		Avatar:         req.Avatar,
		Color:          req.Color,
		Skills:         req.Skills,
	})
	if writeDBError(w, err, "agent not found") {
		return
	}
	s.logger.Info("agent updated", "agent", agent.Name, "id", agent.ID)
	writeJSON(w, http.StatusOK, agent)
}
