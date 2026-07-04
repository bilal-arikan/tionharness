package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/ingest"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
	"github.com/bilal-arikan/swarmgo/internal/settings"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

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
		Shared: req.Shared, SlugPrefix: req.SlugPrefix, Group: req.Group,
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
	// Default-on: a pack that omits mcpEnabled (or ships it false because Go's
	// bool zero value is false) is treated as "tools on" so the fresh agent
	// can actually do work without the user having to flip the master switch
	// in agent detail. To turn tools off for a chat-only agent, use
	// UpdateAgentTools after install — see db.Agent.MCPEnabled for rationale
	// (the DB layer does not default this because booleans cannot tell "unset"
	// from "explicit false").
	mcpEnabled := ap.MCPEnabled
	if !mcpEnabled {
		mcpEnabled = true
	}
	created, err := wsp.DB.CreateAgent(r.Context(), db.Agent{
		Name:           ap.Name,
		Soul:           ap.Soul,
		Identity:       ap.Identity,
		Provider:       ap.Provider,
		Model:          ap.Model,
		ThinkingLevel:  ap.ThinkingLevel,
		PermissionMode: ap.PermissionMode,
		Avatar:         ap.Avatar,
		Color:          ap.Color,
		MCPEnabled:     mcpEnabled,
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
		Name: name, Graph: graph,
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
