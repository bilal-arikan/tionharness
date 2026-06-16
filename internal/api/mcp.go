package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/mcp"
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
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	var req createMCPReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Transport == "" {
		req.Transport = db.MCPTransportStdio
	}
	if req.Transport == db.MCPTransportStdio && req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required for stdio transport")
		return
	}

	argsJSON, _ := json.Marshal(req.Args)
	envJSON, _ := json.Marshal(req.Env)
	if req.Env == nil {
		envJSON = []byte("{}")
	}

	server, err := ws(r).DB.CreateMCPServer(r.Context(), db.MCPServer{
		Name:      req.Name,
		Transport: req.Transport,
		Command:   req.Command,
		Args:      string(argsJSON),
		URL:       req.URL,
		EnvConfig: string(envJSON),
		Enabled:   true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, server)
}

type toggleMCPReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleToggleMCPServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req toggleMCPReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
	return mcp.ServerConfig{
		Name:      m.Name,
		Transport: m.Transport,
		Command:   m.Command,
		Args:      args,
		URL:       m.URL,
		Env:       env,
	}
}
