package api

import (
	"context"
	"net/http"
	"sort"
	"strings"

	agentpkg "github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
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
	q := r.URL.Query()
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Optional filters (none by default → legacy full roster).
	state := q.Get("state")
	provider := strings.ToLower(q.Get("provider"))
	model := strings.ToLower(q.Get("model"))
	matches := make([]db.Agent, 0, len(agents))
	for _, a := range agents {
		switch state {
		case "", "enabled":
			if state == "enabled" && a.Deleted {
				continue
			}
		case "disabled":
			if !a.Deleted {
				continue
			}
		default:
			writeError(w, http.StatusBadRequest, "state must be enabled or disabled")
			return
		}
		if provider != "" && !strings.Contains(strings.ToLower(a.Provider), provider) {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(a.Model), model) {
			continue
		}
		matches = append(matches, a)
	}
	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	if field != "" {
		less, err := tools.SortByField(matches, field, asc,
			func(a db.Agent) int64 { return a.UpdatedAt },
			func(a db.Agent) int64 { return a.CreatedAt },
			func(a db.Agent) string { return a.Name },
			func(a db.Agent) string { return a.ID })
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sort.SliceStable(matches, less)
	}
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
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
	// CoordinatorMode makes every session this agent opens a coordinator (see
	// db.Agent.CoordinatorMode); CoordinatorWorkflow optionally pins a recipe slug.
	// Unlike MCPEnabled these default OFF — coordination is opt-in.
	CoordinatorMode     bool   `json:"coordinatorMode"`
	CoordinatorWorkflow string `json:"coordinatorWorkflow"`
	// CoordinatorPrompt is injected only while a session of this agent is
	// coordinating (see db.Agent.CoordinatorPrompt).
	CoordinatorPrompt string `json:"coordinatorPrompt"`
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSONStrict[createAgentReq](w, r)
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

	// A pinned recipe is validated here (unlike at seed/install time, where an
	// unresolvable slug is dropped): this is a direct, interactive edit, so a typo
	// must come back as an error rather than silently becoming free coordination.
	if req.CoordinatorWorkflow != "" {
		if _, err := ResolveCoordinatorRecipe(ws(r).Runtime.Skills(), req.CoordinatorWorkflow); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// req.Provider is accepted as a provider INSTANCE id (_Docs/71 §5): for every
	// default (migrated) instance the id equals its kind id, so this is
	// byte-for-byte the historical request shape. SyncProviderFields resolves the
	// kind and keeps Agent.Provider/ProviderInstanceID in lockstep (K3).
	providerKind, providerInstanceID, err := agentpkg.SyncProviderFields(s.providers, req.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	newAgent, err := ws(r).DB.CreateAgent(r.Context(), db.Agent{
		Name:                req.Name,
		Soul:                req.Soul,
		Identity:            req.Identity,
		Provider:            providerKind,
		ProviderInstanceID:  providerInstanceID,
		Model:               req.Model,
		ThinkingLevel:       req.ThinkingLevel,
		PermissionMode:      req.PermissionMode,
		MCPEnabled:          mcpEnabled,
		CoordinatorMode:     req.CoordinatorMode,
		CoordinatorWorkflow: req.CoordinatorWorkflow,
		CoordinatorPrompt:   req.CoordinatorPrompt,
	})
	if writeDBError(w, err, "") {
		return
	}

	if req.Avatar != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), newAgent.ID, db.AgentProfilePatch{Avatar: &req.Avatar})
		newAgent.Avatar = req.Avatar
	}
	if req.Color != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), newAgent.ID, db.AgentProfilePatch{Color: &req.Color})
		newAgent.Color = req.Color
	}

	s.logger.Info("agent created", "agent", newAgent.Name, "id", newAgent.ID,
		"provider", newAgent.Provider, "model", newAgent.Model)
	writeJSON(w, http.StatusCreated, newAgent)
}

// handleDuplicateAgent creates a full copy of an existing agent: every profile
// field, provider/model, thinking + permission mode, visual identity, skills and
// the whole tool-access configuration (MCPEnabled + tool overrides / allow +
// block lists) are carried over verbatim. Only identity fields are reset — the
// clone gets a fresh ID (assigned by CreateAgent), a "(kopya)" name suffix, and
// clean created/updated/deleted state. It is NOT marked as agent-created, so the
// user keeps full edit/delete rights over the copy.
func (s *Server) handleDuplicateAgent(w http.ResponseWriter, r *http.Request) {
	src, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}

	clone := src
	clone.ID = "" // CreateAgent assigns a new prefixed id
	clone.Name = src.Name + " (kopya)"
	clone.CreatedBy = ""  // user-owned copy, not an agent-created entity
	clone.Deleted = false // never inherit the deleted flag
	clone.DeletedAt = 0
	clone.CreatedAt = 0 // stamped by CreateAgent
	clone.UpdatedAt = 0

	agent, err := ws(r).DB.CreateAgent(r.Context(), clone)
	if writeDBError(w, err, "") {
		return
	}

	s.logger.Info("agent duplicated", "source", src.ID, "clone", agent.ID, "name", agent.Name)
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

// agentRunning reports whether the agent has work in flight right now, delegating
// to Runtime.AgentBusy — the single implementation both delete paths share (this
// endpoint and the self-management delete_agent tool). The runtime sees the
// interactive chat runs through the probe installed in NewServer.
//
// A workspace with no runtime yet (early boot) falls back to the chat-run
// registry alone rather than reporting "idle": answering "not busy" without
// having looked is exactly the failure the guard exists to prevent.
func (s *Server) agentRunning(ctx context.Context, wsp *workspace.Workspace, agentID string) (bool, string) {
	if wsp.Runtime != nil {
		return wsp.Runtime.AgentBusy(ctx, agentID)
	}
	for _, sid := range s.runs.activeSessionIDs(wsp.ID) {
		sess, err := wsp.DB.GetSession(ctx, sid)
		if err != nil {
			continue // vanished between the snapshot and the lookup
		}
		if sess.AgentID == agentID {
			return true, sess.ID
		}
		for _, p := range sess.Participants {
			if p == agentID {
				return true, sess.ID
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
	// Coordinator defaults for NEW sessions of this agent. Pointers so omitting
	// them leaves the current setting alone and an explicit false turns it off.
	// Existing sessions keep whatever mode they are already in — the toggle is a
	// default, not a broadcast (see db.Agent.CoordinatorMode).
	CoordinatorMode     *bool   `json:"coordinatorMode"`
	CoordinatorWorkflow *string `json:"coordinatorWorkflow"`
	// CoordinatorPrompt is the coordinator-only prompt block. Pointer so omitting
	// it leaves the stored text alone and an explicit "" clears it.
	CoordinatorPrompt *string `json:"coordinatorPrompt"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSONStrict[updateAgentReq](w, r)
	if !ok {
		return
	}
	if req.Name != nil && *req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	wsp := ws(r)
	agentID := r.PathValue("id")

	// Same rule as create: an interactive edit naming an unknown recipe is an
	// error, not a silent downgrade to free coordination. Clearing it ("") is fine.
	if req.CoordinatorWorkflow != nil && *req.CoordinatorWorkflow != "" {
		if _, err := ResolveCoordinatorRecipe(wsp.Runtime.Skills(), *req.CoordinatorWorkflow); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Snapshot the agent before the update so we can detect a model change and
	// emit an event (P1.2). Harmless when the read fails — we skip the event.
	prev, _ := wsp.DB.GetAgent(r.Context(), agentID)

	patch := db.AgentProfilePatch{
		Name:                req.Name,
		Soul:                req.Soul,
		Identity:            req.Identity,
		Model:               req.Model,
		ThinkingLevel:       req.ThinkingLevel,
		PermissionMode:      req.PermissionMode,
		Avatar:              req.Avatar,
		Color:               req.Color,
		Skills:              req.Skills,
		CoordinatorMode:     req.CoordinatorMode,
		CoordinatorWorkflow: req.CoordinatorWorkflow,
		CoordinatorPrompt:   req.CoordinatorPrompt,
	}
	// req.Provider is accepted as a provider INSTANCE id (_Docs/71 §5); only sync
	// when the request actually touches it (nil = "not in this patch").
	if req.Provider != nil {
		providerKind, providerInstanceID, err := agentpkg.SyncProviderFields(s.providers, *req.Provider)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		patch.Provider = &providerKind
		patch.ProviderInstanceID = &providerInstanceID
	}

	agent, err := wsp.DB.UpdateAgent(r.Context(), agentID, patch)
	if writeDBError(w, err, "agent not found") {
		return
	}
	s.logger.Info("agent updated", "agent", agent.Name, "id", agent.ID)

	// --- P1.2: model-change event ---
	if req.Model != nil && *req.Model != prev.Model {
		s.publishAgentModelChange(wsp, tools.AgentModelChange{AgentID: agent.ID, AgentName: agent.Name, OldModel: prev.Model, NewModel: *req.Model})
	}

	// --- P1.3: unknown model warning ---
	resp := map[string]any{"agent": agent}
	if req.Model != nil {
		if warning := tools.AgentModelChangeWarning(agent); warning != "" {
			resp["warning"] = warning
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
