package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := ws(r).DB.ListMCPServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if servers == nil {
		servers = []db.MCPServer{}
	}
	writeJSON(w, http.StatusOK, servers)
}

type createMCPReq struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	Env       map[string]string `json:"env"`
	Headers   map[string]string `json:"headers"`
	// Scope: "shared" (default) reuses one workspace-wide connection; "scoped"
	// gives each (session, agent) its own idle-evicted connection. Empty ⇒ shared.
	Scope string `json:"scope"`
}

// toMCPRow validates a create request and renders the stored row (Enabled=true).
// It does not persist — callers (single create + bulk import) share it.
func (req createMCPReq) toMCPRow() (db.MCPServer, error) {
	if req.Name == "" {
		return db.MCPServer{}, fmt.Errorf("name is required")
	}
	if req.Transport == "" {
		req.Transport = db.MCPTransportStdio
	}
	switch req.Transport {
	case db.MCPTransportStdio:
		if req.Command == "" {
			return db.MCPServer{}, fmt.Errorf("command is required for stdio transport")
		}
	case db.MCPTransportHTTP:
		if req.URL == "" {
			return db.MCPServer{}, fmt.Errorf("url is required for http transport")
		}
	case db.MCPTransportSSE:
		return db.MCPServer{}, fmt.Errorf("the sse transport is deprecated and unsupported — use http (Streamable HTTP)")
	default:
		return db.MCPServer{}, fmt.Errorf("unknown transport %q", req.Transport)
	}

	// Persist an empty args slice as "[]" (never nil → "null"): the frontend does
	// JSON.parse(args).join(...) and a "null" would crash it ("Cannot read
	// properties of null (reading 'join')").
	if req.Args == nil {
		req.Args = []string{}
	}
	argsJSON, _ := json.Marshal(req.Args)
	envJSON := []byte("{}")
	if req.Env != nil {
		envJSON, _ = json.Marshal(req.Env)
	}
	headersJSON := []byte("{}")
	if req.Headers != nil {
		headersJSON, _ = json.Marshal(req.Headers)
	}
	// Validate scope explicitly rather than silently coercing an unknown value —
	// a typo should surface, not quietly become "shared". Empty ⇒ store default.
	switch req.Scope {
	case "", "shared", "scoped":
	default:
		return db.MCPServer{}, fmt.Errorf("unknown scope %q (want \"shared\" or \"scoped\")", req.Scope)
	}
	return db.MCPServer{
		Name:          req.Name,
		Transport:     req.Transport,
		Command:       req.Command,
		Args:          string(argsJSON),
		URL:           req.URL,
		EnvConfig:     string(envJSON),
		HeadersConfig: string(headersJSON),
		Enabled:       true,
		Scope:         req.Scope,
	}, nil
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createMCPReq](w, r)
	if !ok {
		return
	}
	row, err := req.toMCPRow()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	server, err := ws(r).DB.CreateMCPServer(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, server)
}

// handleUpdateMCPServer edits an existing server (PATCH). Body is the same shape
// as create; identity (id, createdAt, createdBy) and enabled state are preserved.
// A changed connection spec (incl. scope) re-dials on the next turn.
func (s *Server) handleUpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[createMCPReq](w, r)
	if !ok {
		return
	}
	row, err := req.toMCPRow()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := ws(r).DB.UpdateMCPServer(r.Context(), id, row)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("mcp server updated", "id", id, "name", updated.Name, "scope", updated.Scope)
	writeJSON(w, http.StatusOK, updated)
}

// mcpPoolStatsResp is the live-connection snapshot for the Tools screen: per-
// server aggregates plus the reaper's idle window.
type mcpPoolStatsResp struct {
	IdleSec int                `json:"idleSec"` // scoped idle-eviction window (0 = disabled)
	Servers []mcpPoolServerAgg `json:"servers"`
}

type mcpPoolServerAgg struct {
	Server string `json:"server"` // server name
	Live   int    `json:"live"`   // alive connections right now
	Total  int    `json:"total"`  // pool entries (alive or reconnecting)
	Scoped bool   `json:"scoped"` // has at least one per-(session,agent) connection
}

// handleMCPPoolStats reports the live pool state so the UI can show how many
// connections a scoped server holds and how close they are to idle eviction.
func (s *Server) handleMCPPoolStats(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	resp := mcpPoolStatsResp{Servers: []mcpPoolServerAgg{}}
	if wsp == nil || wsp.Runtime == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	pool := wsp.Runtime.MCPPool()
	if pool == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.IdleSec = int(pool.IdleTTL().Seconds())
	byServer := map[string]*mcpPoolServerAgg{}
	order := []string{}
	for _, st := range pool.Stats() {
		agg, ok := byServer[st.Server]
		if !ok {
			agg = &mcpPoolServerAgg{Server: st.Server}
			byServer[st.Server] = agg
			order = append(order, st.Server)
		}
		agg.Total++
		if st.Alive {
			agg.Live++
		}
		if st.Scoped {
			agg.Scoped = true
		}
	}
	for _, name := range order {
		resp.Servers = append(resp.Servers, *byServer[name])
	}
	writeJSON(w, http.StatusOK, resp)
}

// importMCPServerSpec is one entry in the standard mcpServers JSON object (the
// shape Claude Code / .mcp.json use). Transport is inferred from "type", else
// from whether a command or url is present.
type importMCPServerSpec struct {
	Type    string            `json:"type"` // stdio | http (sse rejected)
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// importMCPReq accepts either a top-level {"mcpServers": {...}} document or a bare
// {...} map of name→spec, so the operator can paste either form.
type importMCPReq struct {
	MCPServers map[string]importMCPServerSpec `json:"mcpServers"`
}

// handleImportMCPServers bulk-creates MCP servers from a pasted mcpServers JSON
// document. Each entry is validated and created independently; per-entry failures
// are reported without aborting the rest, so one bad spec does not lose the batch.
func (s *Server) handleImportMCPServers(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	// Accept both {"mcpServers": {...}} and a bare {name: spec} map.
	var doc importMCPReq
	if err := json.Unmarshal(raw, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	specs := doc.MCPServers
	if specs == nil {
		var bare map[string]importMCPServerSpec
		if err := json.Unmarshal(raw, &bare); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		specs = bare
	}
	if len(specs) == 0 {
		writeError(w, http.StatusBadRequest, "no mcpServers entries found")
		return
	}

	created := []db.MCPServer{}
	errs := map[string]string{}
	for name, spec := range specs {
		req := createMCPReq{
			Name:      name,
			Transport: inferTransport(spec),
			Command:   spec.Command,
			Args:      spec.Args,
			URL:       spec.URL,
			Env:       spec.Env,
			Headers:   spec.Headers,
		}
		row, err := req.toMCPRow()
		if err != nil {
			errs[name] = err.Error()
			continue
		}
		server, err := ws(r).DB.CreateMCPServer(r.Context(), row)
		if err != nil {
			errs[name] = err.Error()
			continue
		}
		created = append(created, server)
	}
	s.logger.Info("mcp servers imported", "created", len(created), "failed", len(errs))
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "errors": errs})
}

// inferTransport derives the transport from an import spec: explicit "type" wins,
// otherwise a command implies stdio and a url implies http.
func inferTransport(spec importMCPServerSpec) string {
	if spec.Type != "" {
		return spec.Type
	}
	if spec.Command != "" {
		return db.MCPTransportStdio
	}
	if spec.URL != "" {
		return db.MCPTransportHTTP
	}
	return db.MCPTransportStdio
}

type toggleMCPReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleToggleMCPServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[toggleMCPReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetMCPServerEnabled(r.Context(), id, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("mcp server toggled", "id", id, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

func (s *Server) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if err := ws(r).DB.DeleteMCPServer(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})
}

// handleTestMCPServer connects to a stored server and lists its tools, so the
// operator can verify the connection from the UI before assigning it.
func (s *Server) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	server, err := ws(r).DB.GetMCPServer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "mcp server not found")
		return
	}

	cfg := mcpServerConfig(server)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	entries, _, errs := ws(r).Runtime.MCPPool().Catalog(ctx, []mcp.ServerConfig{cfg})
	if errText := errs[server.Name]; errText != "" {
		s.logger.Warn("mcp server test failed", "server", server.Name, "error", errText)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": errText})
		return
	}
	tools := make([]mcp.Tool, 0, len(entries))
	for _, entry := range entries {
		tools = append(tools, entry.Tool)
	}
	s.logger.Info("mcp server tested", "server", server.Name, "tools", len(tools))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "toolCount": len(tools), "tools": tools})
}

// importableMCPServer is one MCP server configured in ANOTHER workspace, offered
// for one-click copy into the current workspace.
type importableMCPServer struct {
	WorkspaceID   string       `json:"workspaceId"`
	WorkspaceName string       `json:"workspaceName"`
	Server        db.MCPServer `json:"server"`
}

// mcpSignature identifies a server by its wiring (name + transport + command +
// args + url), so the same server present in several workspaces dedupes to one
// entry and matches against the current workspace's set.
func mcpSignature(m db.MCPServer) string {
	return strings.Join([]string{
		strings.TrimSpace(strings.ToLower(m.Name)),
		m.Transport, m.Command, m.Args, m.URL,
	}, "\x00")
}

// handleImportableMCPServers lists MCP servers configured in OTHER workspaces that
// are not already present (by wiring signature) in the current one, so the UI can
// offer them for one-click copy. Deduped across workspaces (first occurrence wins).
func (s *Server) handleImportableMCPServers(w http.ResponseWriter, r *http.Request) {
	cur := ws(r)
	ctx := r.Context()
	// Signatures already in THIS workspace — never offer a duplicate.
	have := map[string]bool{}
	if servers, err := cur.DB.ListMCPServers(ctx); err == nil {
		for _, m := range servers {
			have[mcpSignature(m)] = true
		}
	}
	out := []importableMCPServer{}
	for _, meta := range s.workspaces.List() {
		if meta.ID == cur.ID {
			continue
		}
		other, err := s.workspaces.Get(meta.ID)
		if err != nil {
			continue
		}
		servers, err := other.DB.ListMCPServers(ctx)
		if err != nil {
			continue
		}
		for _, m := range servers {
			sig := mcpSignature(m)
			if have[sig] {
				continue
			}
			have[sig] = true // also dedupe across the remaining workspaces
			out = append(out, importableMCPServer{
				WorkspaceID: meta.ID, WorkspaceName: meta.Name, Server: m,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type addImportableReq struct {
	WorkspaceID string `json:"workspaceId"`
	ServerID    string `json:"serverId"`
}

// handleAddImportableMCPServer copies a server from another workspace into the
// current one as a fresh user-owned row (new id, enabled). Its tools appear on the
// next agent turn, exactly like any other newly added server.
func (s *Server) handleAddImportableMCPServer(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[addImportableReq](w, r)
	if !ok {
		return
	}
	src, err := s.workspaces.Get(req.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source workspace not found")
		return
	}
	m, err := src.DB.GetMCPServer(r.Context(), req.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source server not found")
		return
	}
	created, err := ws(r).DB.CreateMCPServer(r.Context(), db.MCPServer{
		Name:          m.Name,
		Description:   m.Description,
		Transport:     m.Transport,
		Command:       m.Command,
		Args:          m.Args,
		URL:           m.URL,
		EnvConfig:     m.EnvConfig,
		HeadersConfig: m.HeadersConfig,
		Enabled:       true,
		Scope:         m.Scope, // preserve the source server's scope (store defaults "" ⇒ shared)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("mcp server imported from workspace", "from", req.WorkspaceID, "name", created.Name, "id", created.ID)
	writeJSON(w, http.StatusCreated, created)
}

// mcpServerConfig converts a db row to an mcp.ServerConfig (mirrors the agent
// package helper, kept local to avoid an api→agent import).
func mcpServerConfig(m db.MCPServer) mcp.ServerConfig {
	var args []string
	_ = json.Unmarshal([]byte(m.Args), &args)
	env := map[string]string{}
	_ = json.Unmarshal([]byte(m.EnvConfig), &env)
	headers := map[string]string{}
	_ = json.Unmarshal([]byte(m.HeadersConfig), &headers)
	return mcp.ServerConfig{
		Name:      m.Name,
		Transport: m.Transport,
		Command:   m.Command,
		Args:      args,
		URL:       m.URL,
		Env:       env,
		Headers:   headers,
	}
}
