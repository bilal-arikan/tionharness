package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/ingest"
	"github.com/bilal-arikan/tionswarm/internal/market"
)

// registerIngestRoutes wires the generic import pipeline: scan a foreign source
// (GitHub repo/plugin or local folder), then bulk-install the selected artifacts
// (skills/agents/commands/MCP) through the same install authority the market uses.
func (s *Server) registerIngestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/ingest/scan", s.handleIngestScan)
	mux.HandleFunc("POST /api/ingest/preview", s.handleIngestPreview)
	mux.HandleFunc("POST /api/ingest/install", s.handleIngestInstall)
}

// handleIngestPreview fetches a source and returns its first artifacts WITH rendered
// bodies, so a source-ref (directory-site) catalog entry can be previewed before install.
func (s *Server) handleIngestPreview(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[ingestSource](w, r)
	if !ok {
		return
	}
	source, location := req.resolve()
	if location == "" {
		writeError(w, http.StatusBadRequest, "path (local) or url (github) is required")
		return
	}
	items, err := ingest.Preview(source, location)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ingestSource is the shared source descriptor for scan/install.
type ingestSource struct {
	Source string `json:"source"` // "github" (default) | "local"
	Path   string `json:"path"`   // local folder tree (source=local)
	URL    string `json:"url"`    // github repo/tree URL or owner/repo (source=github)
}

func (req ingestSource) resolve() (source, location string) {
	source = req.Source
	if source == "" {
		source = "github"
	}
	if source == "local" {
		return source, strings.TrimSpace(req.Path)
	}
	return source, strings.TrimSpace(req.URL)
}

// handleIngestScan discovers every importable artifact in a source and returns
// preview metadata (kind, slug, name, description, bundled files, warnings, and
// whether it already exists here) so the UI can let the user pick what to import.
func (s *Server) handleIngestScan(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[ingestSource](w, r)
	if !ok {
		return
	}
	source, location := req.resolve()
	if location == "" {
		writeError(w, http.StatusBadRequest, "path (local) or url (github) is required")
		return
	}
	res, err := ingest.Scan(source, location)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.decorateExisting(r, res.Items)
	writeJSON(w, http.StatusOK, res)
}

// decorateExisting marks each discovered item whose target entity already exists in
// this workspace, so the UI can grey it out and skip it by default.
func (s *Server) decorateExisting(r *http.Request, items []ingest.Discovered) {
	wsp := ws(r)
	skillStore := wsp.Runtime.Skills()
	agents, _ := wsp.DB.ListAgents(r.Context())
	mcps, _ := wsp.DB.ListMCPServers(r.Context())
	hooks, _ := wsp.DB.ListHooks(r.Context())
	agentSet := lowerSet(agentNames(agents))
	mcpSet := lowerSet(mcpNames(mcps))
	for i := range items {
		switch items[i].Kind {
		case market.KindSkill:
			_, ok := skillStore.Get(items[i].Slug)
			items[i].Exists = ok
		case market.KindAgent:
			items[i].Exists = agentSet[strings.ToLower(strings.TrimSpace(items[i].Name))]
		case market.KindMCP:
			items[i].Exists = mcpSet[strings.ToLower(strings.TrimSpace(items[i].Name))]
		case market.KindHook:
			items[i].Exists = hookAlreadyInstalled(hooks, items[i].Name)
		}
	}
}

// hookAlreadyInstalled reports whether a discovered hook (named "<Event>:
// <label>") matches an existing workspace hook — same event and a command that
// still references the same script label. Best-effort dedup so re-importing a
// package doesn't stack duplicate hooks.
func hookAlreadyInstalled(existing []db.Hook, discoveredName string) bool {
	event, label, ok := strings.Cut(discoveredName, ": ")
	if !ok {
		return false
	}
	event = strings.TrimSpace(event)
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	for _, h := range existing {
		if h.Event == event && strings.Contains(h.Command, label) {
			return true
		}
	}
	return false
}

// ingestInstallReq is the bulk-install payload: the source plus the selected item
// keys (empty = all), an optional slug prefix to namespace the import, and whether
// to advertise imported skills on-demand.
type ingestInstallReq struct {
	ingestSource
	Keys       []string `json:"keys"`       // selected Discovered.Key values (empty = all)
	SlugPrefix string   `json:"slugPrefix"` // optional namespace prefix
	Shared     bool     `json:"shared"`     // advertise imported skills on-demand
	Group      string   `json:"group"`      // optional Skills-UI group for imported skills
}

// ingestInstallResult aggregates a bulk import: installed entities, skipped items
// (build errors or install conflicts) and any warnings.
type ingestInstallResult struct {
	Message   string                 `json:"message"`
	Installed []market.InstallResult `json:"installed"`
	Skipped   []ingest.SkipNote      `json:"skipped"`
	Warnings  []string               `json:"warnings"`
}

// handleIngestInstall builds packs for the selected artifacts and installs each one
// through the shared install authority (installPackInto). A per-item failure (build
// error or conflict) is recorded as a skip, never aborting the batch. Successful
// installs stamp the market ledger so re-imports show as "Kuruldu"/"Güncelle".
func (s *Server) handleIngestInstall(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[ingestInstallReq](w, r)
	if !ok {
		return
	}
	source, location := req.resolve()
	if location == "" {
		writeError(w, http.StatusBadRequest, "path (local) or url (github) is required")
		return
	}
	packs, skipped, warnings, err := ingest.BuildPacks(source, location, req.Keys, ingest.Options{
		Shared: req.Shared, SlugPrefix: req.SlugPrefix, Group: req.Group,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wsp := ws(r)
	out := ingestInstallResult{Skipped: skipped, Warnings: warnings}
	for _, pack := range packs {
		res, ierr := s.installPackInto(r, wsp, pack, installRequest{})
		if ierr != nil {
			out.Skipped = append(out.Skipped, ingest.SkipNote{
				Key: pack.Kind + ":" + pack.ID, Slug: pack.Name, Reason: ierr.Error(),
			})
			continue
		}
		wsp.Runtime.Market().RecordInstall(pack.ID, pack.Version)
		out.Installed = append(out.Installed, res)
	}
	out.Message = fmt.Sprintf("%d öğe içe aktarıldı", len(out.Installed))
	if len(out.Skipped) > 0 {
		out.Message += fmt.Sprintf(" (%d atlandı)", len(out.Skipped))
	}
	writeJSON(w, http.StatusCreated, out)
}

// lowerSet builds a set of lower-cased trimmed names for existence checks.
func lowerSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return out
}
