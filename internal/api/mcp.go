package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return db.MCPServer{
		Name:          req.Name,
		Transport:     req.Transport,
		Command:       req.Command,
		Args:          string(argsJSON),
		URL:           req.URL,
		EnvConfig:     string(envJSON),
		HeadersConfig: string(headersJSON),
		Enabled:       true,
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

	tools, err := mcp.ListServerTools(ctx, cfg)
	if err != nil {
		s.logger.Warn("mcp server test failed", "server", server.Name, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.logger.Info("mcp server tested", "server", server.Name, "tools", len(tools))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "toolCount": len(tools), "tools": tools})
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
