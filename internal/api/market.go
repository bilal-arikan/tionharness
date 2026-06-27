package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/ingest"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
	"github.com/bilal-arikan/swarmgo/internal/settings"
	"github.com/bilal-arikan/swarmgo/internal/skills"
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
	mux.HandleFunc("GET /api/market/connectors", s.handleListConnectors)
	mux.HandleFunc("GET /api/market/connectors/search", s.handleSearchConnectors)
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

// handleListConnectors returns the built-in directory-site connectors (skillsmp …)
// for the UI's quick-add list.
func (s *Server) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, market.ListConnectors())
}

// connectorSearchResult is the search response: matching source-ref packs plus any
// per-connector warnings (one site down doesn't fail the whole search).
type connectorSearchResult struct {
	Results  []market.Pack `json:"results"`
	Warnings []string      `json:"warnings"`
}

// handleSearchConnectors queries the built-in directory-site connectors (skillsmp,
// crossaitools) for a term and returns matching skills as source-ref packs. The
// directory sites hold thousands of skills, so they are searched live rather than
// bulk-listed in the catalog.
func (s *Server) handleSearchConnectors(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, connectorSearchResult{Results: []market.Pack{}})
		return
	}
	limit := 40
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	results, warnings, err := ws(r).Runtime.Market().SearchConnectors(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if results == nil {
		results = []market.Pack{}
	}
	writeJSON(w, http.StatusOK, connectorSearchResult{Results: results, Warnings: warnings})
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
	// Used only by source-ref (directory-site) installs that run the ingest pipeline.
	Shared     bool   `json:"shared"`
	SlugPrefix string `json:"slugPrefix"`
}

// handleInstallMarketPack installs a pack into the workspace via the shared install
// authority, writes the result and stamps the install ledger on success.
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
	// A source-ref pack (directory-site bridge) installs by running the ingest
	// pipeline against its GitHub source, not by writing a single payload.
	if pack.SourceRef != nil {
		s.installSourceRefPack(w, r, wsp, pack, req)
		return
	}
	res, err := s.installPackInto(r, wsp, pack, req)
	if err != nil {
		writeError(w, statusOf(err), err.Error())
		return
	}
	wsp.Runtime.Market().RecordInstall(pack.ID, pack.Version)
	writeJSON(w, http.StatusOK, res)
}

// installSourceRefPack installs a directory-site catalog entry by ingesting its
// GitHub source (the same fetch→adapt→pack→installPackInto pipeline the import
// dialog uses), then stamps the registry pack id in the ledger so the catalog shows
// "Kuruldu". Per-artifact failures are reported as skips, never fatal.
func (s *Server) installSourceRefPack(w http.ResponseWriter, r *http.Request, wsp *workspace.Workspace, pack market.Pack, req installRequest) {
	src := pack.SourceRef
	if src.Type != "" && !strings.EqualFold(src.Type, "github") {
		writeError(w, http.StatusBadRequest, "unsupported source type: "+src.Type)
		return
	}
	packs, skipped, warnings, err := ingest.BuildPacks("github", src.URL, src.Keys, ingest.Options{
		Shared: req.Shared, SlugPrefix: req.SlugPrefix,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := ingestInstallResult{Skipped: skipped, Warnings: warnings}
	for _, p := range packs {
		res, ierr := s.installPackInto(r, wsp, p, installRequest{})
		if ierr != nil {
			out.Skipped = append(out.Skipped, ingest.SkipNote{Key: p.Kind + ":" + p.ID, Slug: p.Name, Reason: ierr.Error()})
			continue
		}
		out.Installed = append(out.Installed, res)
	}
	if len(out.Installed) > 0 {
		wsp.Runtime.Market().RecordInstall(pack.ID, pack.Version)
	}
	out.Message = fmt.Sprintf("%d öğe içe aktarıldı", len(out.Installed))
	if len(out.Skipped) > 0 {
		out.Message += fmt.Sprintf(" (%d atlandı)", len(out.Skipped))
	}
	writeJSON(w, http.StatusCreated, out)
}

// installPackInto is the SINGLE install authority: it routes a pack to its per-kind
// installer and returns the result or an error. Shared by the market install
// endpoint AND the generic ingest import endpoint — so importing a foreign repo and
// installing a native pack land in exactly the same place, per entity kind. Each
// kind writes to its own store: skill → workspace skills dir (+catalog reload);
// agent → db.CreateAgent (provenance cleared, unknown skills dropped); flow →
// db.CreateFlow (empty agent slots auto-assigned); provider → UpsertCustomProvider;
// mcp/workspace/memory likewise.
func (s *Server) installPackInto(r *http.Request, wsp *workspace.Workspace, pack market.Pack, req installRequest) (market.InstallResult, error) {
	switch pack.Kind {
	case market.KindSkill:
		res, err := market.InstallSkill(pack, wsp.Runtime.WorkspaceSkillsDir(), req.Overwrite)
		if err != nil {
			return res, httpErr{http.StatusConflict, err.Error()}
		}
		wsp.Runtime.Skills().Reload() // surface the new skill immediately
		return res, nil
	case market.KindAgent:
		return s.installAgentPack(r, wsp, pack)
	case market.KindFlow:
		return s.installFlowPack(r, wsp, pack)
	case market.KindProvider:
		return s.installProviderPack(wsp, pack, req.APIKey)
	case market.KindMCP:
		return s.installMCPPack(r, wsp, pack)
	case market.KindWorkspace:
		return s.installWorkspacePack(pack)
	case market.KindMemory:
		return s.installMemoryPack(r, wsp, pack, req.AgentID)
	default:
		return market.InstallResult{}, httpErr{http.StatusNotImplemented, "installing " + pack.Kind + " packs is not yet supported"}
	}
}

// httpErr carries an HTTP status alongside an install error so the market endpoint
// can preserve precise codes (409 conflict, 400 bad payload…) while the ingest
// batch path treats any error simply as a per-item skip.
type httpErr struct {
	code int
	msg  string
}

func (e httpErr) Error() string { return e.msg }

// statusOf extracts the HTTP status from an httpErr, defaulting to 400.
func statusOf(err error) int {
	if he, ok := err.(httpErr); ok && he.code != 0 {
		return he.code
	}
	return http.StatusBadRequest
}

// installMCPPack registers an MCP tool server from a pack payload. The server is
// created enabled; the persistent MCP pool picks it up on the next turn. Dedup is
// by name so re-installing the same pack doesn't spawn a duplicate.
func (s *Server) installMCPPack(r *http.Request, wsp *workspace.Workspace, pack market.Pack) (market.InstallResult, error) {
	mp := pack.Payload.MCP
	if mp == nil || mp.Name == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "MCP pack is missing its payload"}
	}
	if existing, _ := wsp.DB.ListMCPServers(r.Context()); nameExists(mp.Name, mcpNames(existing)) {
		return market.InstallResult{}, httpErr{http.StatusConflict, "\"" + mp.Name + "\" adlı MCP sunucusu zaten kurulu"}
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
		return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
	}
	return market.InstallResult{
		Kind: market.KindMCP, Ref: created.ID,
		Message: "MCP server \"" + created.Name + "\" installed",
	}, nil
}

// installWorkspacePack creates a brand-new workspace from a template pack and
// applies its identity/instructions/board layout. Dedup is by workspace name.
func (s *Server) installWorkspacePack(pack market.Pack) (market.InstallResult, error) {
	wp := pack.Payload.Workspace
	if wp == nil || wp.Name == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "workspace pack is missing its payload"}
	}
	for _, m := range s.workspaces.List() {
		if strings.EqualFold(strings.TrimSpace(m.Name), strings.TrimSpace(wp.Name)) {
			return market.InstallResult{}, httpErr{http.StatusConflict, "\"" + wp.Name + "\" adlı workspace zaten var"}
		}
	}
	created, err := s.workspaces.Create(wp.Name, "", "")
	if err != nil {
		return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
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
	// Seed the starter team (agents + flow + disabled schedules) when the template
	// carries one, so a market-installed workspace arrives as ready as one created
	// from the create-workspace picker.
	s.seedWorkspaceTeam(context.Background(), created, *wp)
	return market.InstallResult{
		Kind: market.KindWorkspace, Ref: created.ID,
		Message: "Workspace \"" + wp.Name + "\" oluşturuldu",
	}, nil
}

// installMemoryPack seeds memory entries into a target agent (memory is per-agent).
// agentID selects the target; an empty agentID falls back to the first agent. It
// errors if no agent exists, or if a supplied agentID is unknown.
func (s *Server) installMemoryPack(r *http.Request, wsp *workspace.Workspace, pack market.Pack, agentID string) (market.InstallResult, error) {
	mp := pack.Payload.Memory
	if mp == nil || len(mp.Entries) == 0 {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "memory pack has no entries"}
	}
	agents, _ := wsp.DB.ListAgents(r.Context())
	if len(agents) == 0 {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "önce bir ajan oluştur (bellek girdileri ajana eklenir)"}
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
			return market.InstallResult{}, httpErr{http.StatusBadRequest, "seçili ajan bulunamadı"}
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
	return market.InstallResult{
		Kind: market.KindMemory, Ref: target.ID,
		Message: fmt.Sprintf("%d bellek girdisi \"%s\" ajanına eklendi", n, target.Name),
	}, nil
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
func (s *Server) installAgentPack(r *http.Request, wsp *workspace.Workspace, pack market.Pack) (market.InstallResult, error) {
	ap := pack.Payload.Agent
	if ap == nil || ap.Name == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "agent pack is missing its payload"}
	}
	// Skip if an agent with the same name already exists (don't create duplicates).
	if existing, _ := wsp.DB.ListAgents(r.Context()); nameExists(ap.Name, agentNames(existing)) {
		return market.InstallResult{}, httpErr{http.StatusConflict, "\"" + ap.Name + "\" adlı ajan zaten kurulu"}
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
		return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
	}
	return market.InstallResult{
		Kind: market.KindAgent, Ref: created.ID,
		Message: "Agent \"" + created.Name + "\" installed",
	}, nil
}

// installFlowPack creates a flow from a pack payload. The graph is agent-agnostic
// (empty agentId slots); to make it runnable immediately, empty slots are filled
// with the workspace's first agent (mirrors the template-instantiation rule that
// the engine rejects empty agentIds). The user can reassign in the canvas.
func (s *Server) installFlowPack(r *http.Request, wsp *workspace.Workspace, pack market.Pack) (market.InstallResult, error) {
	fp := pack.Payload.Flow
	if fp == nil || fp.Graph == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "flow pack is missing its graph"}
	}
	wantName := fp.Name
	if wantName == "" {
		wantName = pack.Name
	}
	// Skip if a flow with the same name already exists (don't create duplicates).
	if existing, _ := wsp.DB.ListFlows(r.Context()); nameExists(wantName, flowNames(existing)) {
		return market.InstallResult{}, httpErr{http.StatusConflict, "\"" + wantName + "\" adlı akış zaten kurulu"}
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
		return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
	}
	return market.InstallResult{
		Kind: market.KindFlow, Ref: created.ID,
		Message: "Flow \"" + created.Name + "\" installed",
	}, nil
}

// installProviderPack registers a custom provider from a pack payload and pushes
// it live. The API key (never carried in a pack) is supplied by the user here;
// an empty key still installs (the user can add it later in Settings).
func (s *Server) installProviderPack(wsp *workspace.Workspace, pack market.Pack, apiKey string) (market.InstallResult, error) {
	pp := pack.Payload.Provider
	if pp == nil || pp.BaseURL == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "provider pack is missing its payload"}
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
		Reasoning:    pp.Reasoning,
		PromptCache:  pp.PromptCache,
	}, keyPtr); err != nil {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, err.Error()}
	}
	s.applySettings() // push the new provider into the live registry
	return market.InstallResult{
		Kind: market.KindProvider, Ref: id,
		Message: "Provider \"" + pp.Label + "\" installed",
	}, nil
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
		// Collect bundled resource files (nested dirs) so publish is lossless too.
		files := collectSkillFiles(filepath.Dir(sk.Path))
		pack, err := market.BuildSkillPack(sk.Slug, sk.Name, sk.Description, sk.Icon, sk.Color, string(raw), "", 0, files)
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
	case market.KindWorkspace:
		// Capture the ACTIVE workspace's current state into a portable template
		// pack (the inverse of seeding). SourceID is ignored — the workspace scope
		// comes from the X-Workspace-Id header.
		payload, err := s.buildWorkspaceTemplatePayload(r.Context(), wsp)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(payload.Agents) == 0 {
			writeError(w, http.StatusBadRequest, "şablonlanacak ajan yok — workspace en az bir ajan içermeli")
			return
		}
		cfg := wsp.Settings()
		slug := slugify(wsp.Name)
		if slug == "" {
			slug = strings.ToLower(wsp.ID)
		}
		pack, err := market.BuildWorkspacePack(slug, wsp.Name, "“"+wsp.Name+"” workspace'inden dışa aktarıldı.", cfg.Icon, cfg.Color, "", 0, payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stored, err := wsp.Runtime.Market().Publish(pack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// The template picker reads the server-level market store (separate Store
		// instance over the same global dir); reload it so the freshly published
		// template appears in the create picker immediately.
		s.market.Reload()
		writeJSON(w, http.StatusOK, stored)
	default:
		writeError(w, http.StatusNotImplemented, "publishing "+req.Kind+" is not yet supported")
	}
}

// buildWorkspaceTemplatePayload captures a live workspace's current state into a
// portable WorkspacePayload — the inverse of seedWorkspaceTeam: workspace-tier
// skills, the agent team (with stable local keys), flows (agent ids rewritten to
// "tmpl:<key>"), schedules (wired by key), and identity/instructions/board layout.
// Secrets, sessions, artifacts and other runtime data are never included.
func (s *Server) buildWorkspaceTemplatePayload(ctx context.Context, wsp *workspace.Workspace) (market.WorkspacePayload, error) {
	cfg := wsp.Settings()
	wp := market.WorkspacePayload{
		Name:         wsp.Name,
		Icon:         cfg.Icon,
		Color:        cfg.Color,
		Instructions: cfg.Instructions,
	}
	if len(cfg.BoardColumns) > 0 {
		wp.Columns = fromBoardColumnDefs(cfg.BoardColumns)
	}

	// Agents → template agents with stable local keys (id → key map for wiring).
	agents, err := wsp.DB.ListAgents(ctx)
	if err != nil {
		return wp, err
	}
	idToKey := make(map[string]string, len(agents))
	usedKeys := map[string]bool{}
	for _, a := range agents {
		key := uniqueAgentKey(a.Name, usedKeys)
		idToKey[a.ID] = key
		wp.Agents = append(wp.Agents, market.WorkspaceTemplateAgent{
			Key: key, Name: a.Name, Soul: a.Soul, Identity: a.Identity,
			Provider: a.Provider, Model: a.Model,
			PlanningMode: a.PlanningMode, ThinkingLevel: a.ThinkingLevel, PermissionMode: a.PermissionMode,
			Avatar: a.Avatar, Color: a.Color,
			MCPEnabled: a.MCPEnabled, AllowedTools: a.AllowedTools, BlockedTools: a.BlockedTools,
			Skills:         a.Skills,
			DailyCallLimit: a.DailyCallLimit, DailyTokenLimit: a.DailyTokenLimit,
		})
	}

	// Flows → rewrite agent node ids to "tmpl:<key>" so the graph is portable.
	flows, ferr := wsp.DB.ListFlows(ctx)
	if ferr != nil {
		return wp, ferr
	}
	for _, f := range flows {
		graph, perr := orchestration.ParseGraph(f.Graph)
		if perr != nil {
			continue // skip an unparseable flow rather than failing the whole export
		}
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.Type == orchestration.NodeAgent && n.AgentID != "" {
				if key, ok := idToKey[n.AgentID]; ok {
					n.AgentID = market.TemplateAgentKeyPrefix + key
				}
			}
		}
		raw, merr := json.Marshal(graph)
		if merr != nil {
			continue
		}
		wp.Flows = append(wp.Flows, market.WorkspaceTemplateFlow{
			Name: f.Name, Description: f.Description, Graph: string(raw),
		})
	}

	// Schedules → reference the agent by key (skip orphans).
	scheds, serr := wsp.DB.ListSchedules(ctx)
	if serr != nil {
		return wp, serr
	}
	for _, sc := range scheds {
		key, ok := idToKey[sc.AgentID]
		if !ok {
			continue
		}
		wp.Schedules = append(wp.Schedules, market.WorkspaceTemplateSchedule{
			AgentKey: key, CronExpr: sc.CronExpr, Prompt: sc.Prompt,
		})
	}

	// Workspace-tier skills → embed verbatim (SKILL.md + nested files) so the
	// template is self-contained. Global/bundled skills are shared, not exported.
	for _, sk := range wsp.Runtime.Skills().List() {
		if sk.Source != skills.SourceWorkspace || sk.Path == "" {
			continue
		}
		body, rerr := os.ReadFile(sk.Path)
		if rerr != nil {
			continue
		}
		wp.Skills = append(wp.Skills, market.WorkspaceTemplateSkill{
			Slug: sk.Slug, Body: string(body), Files: collectSkillFiles(filepath.Dir(sk.Path)),
		})
	}

	return wp, nil
}

// fromBoardColumnDefs is the inverse of toBoardColumnDefs: db board columns → the
// market envelope's dependency-free BoardColumn list, for publishing a template.
func fromBoardColumnDefs(in []db.BoardColumnDef) []market.BoardColumn {
	out := make([]market.BoardColumn, 0, len(in))
	for _, c := range in {
		out = append(out, market.BoardColumn{Key: c.Key, Label: c.Label, Color: c.Color})
	}
	return out
}

// uniqueAgentKey derives a stable, unique local key for a template agent from its
// name, deduping with a numeric suffix. Used to wire flows/schedules by key.
func uniqueAgentKey(name string, used map[string]bool) string {
	base := slugify(name)
	if base == "" {
		base = "agent"
	}
	key := base
	for i := 2; used[key]; i++ {
		key = fmt.Sprintf("%s-%d", base, i)
	}
	used[key] = true
	return key
}

// slugify lowercases a string and keeps only [a-z0-9], collapsing runs of other
// characters to single dashes. Non-ASCII (e.g. Turkish) letters are dropped, so
// callers fall back to another id when the result is empty.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// collectSkillFiles reads a skill folder's bundled resource files (every file
// except SKILL.md, nested dirs included) keyed by forward-slashed relative path, so
// publish carries them into the pack. Returns nil when only SKILL.md is present.
func collectSkillFiles(dir string) map[string][]byte {
	files := map[string][]byte{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil || strings.EqualFold(rel, "SKILL.md") {
			return nil
		}
		if b, rderr := os.ReadFile(p); rderr == nil {
			files[filepath.ToSlash(rel)] = b
		}
		return nil
	})
	if len(files) == 0 {
		return nil
	}
	return files
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
