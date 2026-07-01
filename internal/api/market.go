package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/market"
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

// publishRequest packages an existing workspace entity into the local registry.
type publishRequest struct {
	Kind     string `json:"kind"`
	SourceID string `json:"sourceId"` // skill slug / agent id / flow id / provider id
	// Include filters what a workspace-template export captures. A nil pointer
	// (field omitted) means "everything" for backward compatibility.
	Include *publishInclude `json:"include,omitempty"`
}

// publishInclude selects which parts of a live workspace get captured into a
// template pack. It only applies to the workspace kind. A nil AgentIDs slice
// means "all agents"; a present-but-empty slice means "none" (rejected upstream
// since a template needs at least one agent). The boolean flags gate the
// optional categories — when Include is provided they are honoured verbatim, so
// the caller must set every flag it wants included.
type publishInclude struct {
	AgentIDs     []string `json:"agentIds"` // nil = all agents
	Flows        bool     `json:"flows"`
	Schedules    bool     `json:"schedules"`
	Skills       bool     `json:"skills"`
	Instructions bool     `json:"instructions"`
	BoardColumns bool     `json:"boardColumns"`
}
