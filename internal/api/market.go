package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/bilal/swarmgo/internal/market"
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

// handleInstallMarketPack installs a pack into the workspace. MVP: skill packs
// are written into the workspace skills dir and the skill catalog is reloaded.
// Other kinds return 501 until their installers land.
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
	default:
		writeError(w, http.StatusNotImplemented, "installing "+pack.Kind+" packs is not yet supported")
	}
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
