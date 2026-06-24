package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
	"github.com/bilal-arikan/swarmgo/internal/settings"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
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
	// Remote registries (Faz 1-4): list/add/remove/refresh remote pack sources.
	mux.HandleFunc("GET /api/market/registries", s.handleListRegistries)
	mux.HandleFunc("POST /api/market/registries", s.handleAddRegistry)
	mux.HandleFunc("POST /api/market/registries/delete", s.handleRemoveRegistry)
	mux.HandleFunc("POST /api/market/registries/refresh", s.handleRefreshRegistries)
	mux.HandleFunc("GET /api/market/{id}", s.handleGetMarketPack)
	mux.HandleFunc("POST /api/market/{id}/install", s.handleInstallMarketPack)
}

// handleListRegistries returns the configured remote registries.
func (s *Server) handleListRegistries(w http.ResponseWriter, r *http.Request) {
	regs := ws(r).Runtime.Market().ListRegistries()
	if regs == nil {
		regs = []market.Registry{}
	}
	writeJSON(w, http.StatusOK, regs)
}

// registryRequest is the body for adding/removing a registry.
type registryRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// handleAddRegistry adds a remote registry, then refreshes it so its packs appear
// immediately. Add succeeds even if the first refresh fails (it's retryable).
func (s *Server) handleAddRegistry(w http.ResponseWriter, r *http.Request) {
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	store := ws(r).Runtime.Market()
	regs, err := store.AddRegistry(req.Name, req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = store.RefreshRemote(r.Context()) // best-effort initial fetch
	writeJSON(w, http.StatusOK, regs)
}

// handleRemoveRegistry drops a registry by URL and clears its cached index.
func (s *Server) handleRemoveRegistry(w http.ResponseWriter, r *http.Request) {
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	regs, err := ws(r).Runtime.Market().RemoveRegistry(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, regs)
}

// handleRefreshRegistries re-fetches every enabled registry index. Per-registry
// errors are surfaced but the cache for the ones that succeeded is updated.
func (s *Server) handleRefreshRegistries(w http.ResponseWriter, r *http.Request) {
	if err := ws(r).Runtime.Market().RefreshRemote(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	// Decorate each pack with the version last installed here (from the ledger) so
	// the UI can show "Kuruldu" / "Güncelle". Empty = never installed via market.
	led := store.InstalledVersions()
	for i := range list {
		if v, ok := led[list[i].ID]; ok {
			list[i].InstalledVersion = v
		}
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
// an existing entity; apiKey supplies a custom provider's key (provider kind);
// agentID selects the target agent for a memory pack (defaults to the first agent).
type installRequest struct {
	Overwrite bool   `json:"overwrite"`
	APIKey    string `json:"apiKey"`
	AgentID   string `json:"agentId"`
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

	// Capture the response status so we can stamp the install ledger (packID →
	// version) once, for any kind, on success — this drives "Kuruldu"/"Güncelle".
	rec := &statusCaptureWriter{ResponseWriter: w, status: http.StatusOK}
	w = rec

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

	case market.KindMCP:
		s.installMCPPack(w, r, wsp, pack)

	case market.KindWorkspace:
		s.installWorkspacePack(w, pack)

	case market.KindMemory:
		s.installMemoryPack(w, r, wsp, pack, req.AgentID)

	default:
		writeError(w, http.StatusNotImplemented, "installing "+pack.Kind+" packs is not yet supported")
	}

	if rec.status == http.StatusOK {
		wsp.Runtime.Market().RecordInstall(pack.ID, pack.Version)
	}
}

// statusCaptureWriter records the HTTP status written by a handler so the caller
// can branch on success after delegating the response.
type statusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusCaptureWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// installMCPPack registers an MCP tool server from a pack payload. The server is
// created enabled; the persistent MCP pool picks it up on the next turn. Dedup is
// by name so re-installing the same pack doesn't spawn a duplicate.
func (s *Server) installMCPPack(w http.ResponseWriter, r *http.Request, wsp *workspace.Workspace, pack market.Pack) {
	mp := pack.Payload.MCP
	if mp == nil || mp.Name == "" {
		writeError(w, http.StatusBadRequest, "MCP pack is missing its payload")
		return
	}
	if existing, _ := wsp.DB.ListMCPServers(r.Context()); nameExists(mp.Name, mcpNames(existing)) {
		writeError(w, http.StatusConflict, "\""+mp.Name+"\" adlı MCP sunucusu zaten kurulu")
		return
	}
	created, err := wsp.DB.CreateMCPServer(r.Context(), db.MCPServer{
		Name:      mp.Name,
		Transport: mp.Transport,
		Command:   mp.Command,
		Args:      mp.Args,
		URL:       mp.URL,
		EnvConfig: mp.EnvConfig,
		Enabled:   true,
		Scope:     "shared",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindMCP, Ref: created.ID,
		Message: "MCP server \"" + created.Name + "\" installed",
	})
}

// installWorkspacePack creates a brand-new workspace from a template pack and
// applies its identity/instructions/board layout. Dedup is by workspace name.
func (s *Server) installWorkspacePack(w http.ResponseWriter, pack market.Pack) {
	wp := pack.Payload.Workspace
	if wp == nil || wp.Name == "" {
		writeError(w, http.StatusBadRequest, "workspace pack is missing its payload")
		return
	}
	for _, m := range s.workspaces.List() {
		if strings.EqualFold(strings.TrimSpace(m.Name), strings.TrimSpace(wp.Name)) {
			writeError(w, http.StatusConflict, "\""+wp.Name+"\" adlı workspace zaten var")
			return
		}
	}
	created, err := s.workspaces.Create(wp.Name, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	patch := workspace.WSSettingsPatch{}
	if wp.Icon != "" {
		patch.Icon = &wp.Icon
	}
	if wp.Color != "" {
		patch.Color = &wp.Color
	}
	if wp.Instructions != "" {
		patch.Instructions = &wp.Instructions
	}
	if len(wp.Columns) > 0 {
		cols := toBoardColumnDefs(wp.Columns)
		patch.BoardColumns = &cols
	}
	if _, err := s.workspaces.UpdateSettings(created.ID, patch); err != nil {
		s.logger.Warn("workspace pack: apply settings failed", "id", created.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindWorkspace, Ref: created.ID,
		Message: "Workspace \"" + wp.Name + "\" oluşturuldu",
	})
}

// installMemoryPack seeds memory entries into a target agent (memory is per-agent).
// agentID selects the target; an empty agentID falls back to the first agent. It
// errors if no agent exists, or if a supplied agentID is unknown.
func (s *Server) installMemoryPack(w http.ResponseWriter, r *http.Request, wsp *workspace.Workspace, pack market.Pack, agentID string) {
	mp := pack.Payload.Memory
	if mp == nil || len(mp.Entries) == 0 {
		writeError(w, http.StatusBadRequest, "memory pack has no entries")
		return
	}
	agents, _ := wsp.DB.ListAgents(r.Context())
	if len(agents) == 0 {
		writeError(w, http.StatusBadRequest, "önce bir ajan oluştur (bellek girdileri ajana eklenir)")
		return
	}
	target := agents[0]
	if agentID != "" {
		found := false
		for _, a := range agents {
			if a.ID == agentID {
				target = a
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusBadRequest, "seçili ajan bulunamadı")
			return
		}
	}
	n := 0
	for _, e := range mp.Entries {
		if strings.TrimSpace(e.Content) == "" {
			continue
		}
		kind := e.Kind
		if kind == "" {
			kind = db.MemoryDocument
		}
		if _, err := wsp.Runtime.Memory().Remember(r.Context(), target.ID, kind, e.Content); err == nil {
			n++
		}
	}
	writeJSON(w, http.StatusOK, market.InstallResult{
		Kind: market.KindMemory, Ref: target.ID,
		Message: fmt.Sprintf("%d bellek girdisi \"%s\" ajanına eklendi", n, target.Name),
	})
}

// toBoardColumnDefs maps the dependency-free market columns to db.BoardColumnDef.
func toBoardColumnDefs(in []market.BoardColumn) []db.BoardColumnDef {
	out := make([]db.BoardColumnDef, 0, len(in))
	for _, c := range in {
		out = append(out, db.BoardColumnDef{Key: c.Key, Label: c.Label, Color: c.Color})
	}
	return out
}

func mcpNames(in []db.MCPServer) []string {
	out := make([]string, len(in))
	for i, m := range in {
		out[i] = m.Name
	}
	return out
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
