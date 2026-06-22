package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/market"
	"github.com/bilal/swarmgo/internal/orchestration"
	"github.com/bilal/swarmgo/internal/settings"
	"github.com/bilal/swarmgo/internal/workspace"
)

// registerMarketRoutes registers the in-app marketplace: a file-based registry
// of shareable packs (skill/agent/provider/flow) that can be browsed, installed
// into the workspace, and published from existing entities. MVP wires the skill
// kind end-to-end; other kinds list but install/publish lands in later slices.
func (s *Server) registerMarketRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/market", s.handleListMarket)
	mux.HandleFunc("POST /api/market/reload", s.handleReloadMarket)
	mux.HandleFunc("POST /api/market/publish", s.handlePublishMarket)
	mux.HandleFunc("POST /api/market/import", s.handleImportMarket)
	mux.HandleFunc("GET /api/market/{id}", s.handleGetMarketPack)
	mux.HandleFunc("POST /api/market/{id}/install", s.handleInstallMarketPack)
}

// handleListMarket returns the resolved pack catalog (manifests only). An
// optional ?kind= filters to one kind.
func (s *Server) handleListMarket(w http.ResponseWriter, r *http.Request) {
	store := ws(r).Runtime.Market()
	var list []market.Pack
	if kind := r.URL.Query().Get("kind"); kind != "" {
		list = store.ListKind(kind)
	} else {
		list = store.List()
	}
	if list == nil {
		list = []market.Pack{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetMarketPack returns one pack WITH its payload, read fresh from disk.
func (s *Server) handleGetMarketPack(w http.ResponseWriter, r *http.Request) {
	pack, ok := ws(r).Runtime.Market().Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "pack not found")
		return
	}
	writeJSON(w, http.StatusOK, pack)
}

// handleReloadMarket re-scans the registry tiers.
func (s *Server) handleReloadMarket(w http.ResponseWriter, r *http.Request) {
	ws(r).Runtime.Market().Reload()
	w.WriteHeader(http.StatusNoContent)
}

// installRequest is the optional body for an install: overwrite gates clobbering
// an existing entity; apiKey supplies a custom provider's key (provider kind).
type installRequest struct {
	Overwrite bool   `json:"overwrite"`
	APIKey    string `json:"apiKey"`
}

// handleInstallMarketPack installs a pack into the workspace. Each kind lands in
// its own store: skill → workspace skills dir (+catalog reload); agent →
// db.CreateAgent (provenance cleared, unknown skills dropped); flow →
// db.CreateFlow (empty agent slots auto-assigned to the first agent so it runs);
// provider → settings.UpsertCustomProvider (+ live push, optional API key).
func (s *Server) handleInstallMarketPack(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	pack, ok := wsp.Runtime.Market().Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "pack not found")
		return
	}
	var req installRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body optional
	}

	switch pack.Kind {
	case market.KindSkill:
		res, err := market.InstallSkill(pack, wsp.Runtime.WorkspaceSkillsDir(), req.Overwrite)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		wsp.Runtime.Skills().Reload() // surface the new skill immediately
		writeJSON(w, http.StatusOK, res)

	case market.KindAgent:
		s.installAgentPack(w, r, wsp, pack)

	case market.KindFlow:
		s.installFlowPack(w, r, wsp, pack)

	case market.KindProvider:
		s.installProviderPack(w, wsp, pack, req.APIKey)

	default:
		writeError(w, http.StatusNotImplemented, "installing "+pack.Kind+" packs is not yet supported")
	}
}

// installAgentPack creates an agent from a pack payload. Provenance is cleared
// (CreatedBy=""=user) and skills are filtered to those resolvable in this
// workspace so a missing skill never blocks the install.
func (s *Server) installAgentPack(w http.ResponseWriter, r *http.Request, wsp *workspace.Workspace, pack market.Pack) {
	ap := pack.Payload.Agent
	if ap == nil || ap.Name == "" {
		writeError(w, http.StatusBadRequest, "agent pack is missing its payload")
		return
	}
	// Skip if an agent with the same name already exists (don't create duplicates).
	if existing, _ := wsp.DB.ListAgents(r.Context()); nameExists(ap.Name, agentNames(existing)) {
		writeError(w, http.StatusConflict, "\""+ap.Name+"\" adlı ajan zaten kurulu")
		return
	}
	skillStore := wsp.Runtime.Skills()
	known := make([]string, 0, len(ap.Skills))
	for _, slug := range ap.Skills {
		if _, ok := skillStore.Get(slug); ok {
			known = append(known, slug)
		}
	}
	created, err := wsp.DB.CreateAgent(r.Context(), db.Agent{
		Name:           ap.Name,
		Soul:           ap.Soul,
		Identity:       ap.Identity,
		Provider:       ap.Provider,
		Model:          ap.Model,
		PlanningMode:   ap.PlanningMode,
		ThinkingLevel:  ap.ThinkingLevel,
		PermissionMode: ap.PermissionMode,
		Avatar:         ap.Avatar,
		Color:          ap.Color,
		MCPEnabled:     ap.MCPEnabled,
		AllowedTools:   ap.AllowedTools,
		Skills:         known,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindAgent, Ref: created.ID,
		Message: "Agent \"" + created.Name + "\" installed",
	})
}

// installFlowPack creates a flow from a pack payload. The graph is agent-agnostic
// (empty agentId slots); to make it runnable immediately, empty slots are filled
// with the workspace's first agent (mirrors the template-instantiation rule that
// the engine rejects empty agentIds). The user can reassign in the canvas.
func (s *Server) installFlowPack(w http.ResponseWriter, r *http.Request, wsp *workspace.Workspace, pack market.Pack) {
	fp := pack.Payload.Flow
	if fp == nil || fp.Graph == "" {
		writeError(w, http.StatusBadRequest, "flow pack is missing its graph")
		return
	}
	wantName := fp.Name
	if wantName == "" {
		wantName = pack.Name
	}
	// Skip if a flow with the same name already exists (don't create duplicates).
	if existing, _ := wsp.DB.ListFlows(r.Context()); nameExists(wantName, flowNames(existing)) {
		writeError(w, http.StatusConflict, "\""+wantName+"\" adlı akış zaten kurulu")
		return
	}
	graph := fp.Graph
	if g, err := orchestration.ParseGraph(fp.Graph); err == nil {
		if agents, _ := wsp.DB.ListAgents(r.Context()); len(agents) > 0 {
			first := agents[0].ID
			for i := range g.Nodes {
				if g.Nodes[i].Type == orchestration.NodeAgent && g.Nodes[i].AgentID == "" {
					g.Nodes[i].AgentID = first
				}
			}
			if data, mErr := json.Marshal(g); mErr == nil {
				graph = string(data)
			}
		}
	}
	name := fp.Name
	if name == "" {
		name = pack.Name
	}
	created, err := wsp.DB.CreateFlow(r.Context(), db.Flow{
		Name: name, Description: fp.Description, Graph: graph,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindFlow, Ref: created.ID,
		Message: "Flow \"" + created.Name + "\" installed",
	})
}

// installProviderPack registers a custom provider from a pack payload and pushes
// it live. The API key (never carried in a pack) is supplied by the user here;
// an empty key still installs (the user can add it later in Settings).
func (s *Server) installProviderPack(w http.ResponseWriter, wsp *workspace.Workspace, pack market.Pack, apiKey string) {
	pp := pack.Payload.Provider
	if pp == nil || pp.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "provider pack is missing its payload")
		return
	}
	id := providerIDFromPack(pack)
	var keyPtr *string
	if apiKey != "" {
		keyPtr = &apiKey
	}
	if _, err := s.settings.UpsertCustomProvider(settings.CustomProvider{
		ID:           id,
		Label:        pp.Label,
		Kind:         pp.Kind,
		BaseURL:      pp.BaseURL,
		DefaultModel: pp.DefaultModel,
		Models:       pp.Models,
	}, keyPtr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.applySettings() // push the new provider into the live registry
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindProvider, Ref: id,
		Message: "Provider \"" + pp.Label + "\" installed",
	})
}

// providerIDFromPack derives a settings provider id from a pack id. Pack ids are
// "provider.<slug>"; the slug already matches the required id format
// (letter-led, [A-Za-z0-9_.-]).
func providerIDFromPack(pack market.Pack) string {
	id := strings.TrimPrefix(pack.ID, "provider.")
	if id == "" {
		id = pack.ID
	}
	return id
}

// nameExists reports whether want matches any name in the set, case-insensitively
// and trimmed — the dedup test used to avoid creating a second copy of an entity.
func nameExists(want string, names []string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, n := range names {
		if strings.ToLower(strings.TrimSpace(n)) == want {
			return true
		}
	}
	return false
}

func agentNames(in []db.Agent) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = a.Name
	}
	return out
}

func flowNames(in []db.Flow) []string {
	out := make([]string, len(in))
	for i, f := range in {
		out[i] = f.Name
	}
	return out
}

// publishRequest packages an existing workspace entity into the local registry.
type publishRequest struct {
	Kind     string `json:"kind"`
	SourceID string `json:"sourceId"` // skill slug / agent id / flow id / provider id
}

// handlePublishMarket packages an existing entity into the workspace tier of the
// registry. MVP: skill kind (by slug). Other kinds return 501.
func (s *Server) handlePublishMarket(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	wsp := ws(r)
	switch req.Kind {
	case market.KindSkill:
		sk, ok := wsp.Runtime.Skills().Get(req.SourceID)
		if !ok || sk.Path == "" {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		// Carry the full SKILL.md verbatim (frontmatter + body) for a lossless
		// reinstall — read the backing file directly.
		raw, err := os.ReadFile(sk.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		pack, err := market.BuildSkillPack(sk.Slug, sk.Name, sk.Description, sk.Icon, sk.Color, string(raw), "", 0)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stored, err := wsp.Runtime.Market().Publish(pack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stored)
	default:
		writeError(w, http.StatusNotImplemented, "publishing "+req.Kind+" is not yet supported")
	}
}

// handleImportMarket stores a raw pasted SwarmPack JSON into the registry.
func (s *Server) handleImportMarket(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	stored, err := ws(r).Runtime.Market().Import(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stored)
}
