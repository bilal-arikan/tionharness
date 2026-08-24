// Package gateway implements TionHarness's EXTERNAL MCP gateway (Doc 52 Faz 3): a
// streaming MCP-over-HTTP endpoint that exposes TionHarness's backend MCP pool to
// OUTSIDE clients (e.g. another Claude Code / External Agent) behind a single URL, the
// gateway pattern the TS gateway-manager provided. It starts each session with a small
// meta-tool surface (list_servers / activate_tools / deactivate_tools / active_tools);
// activate_tools connects a backend MCP server and registers its tools dynamically,
// pushing tools/list_changed so the client re-lists — keeping the client's context
// minimal until it actually needs a server.
//
// It differs from the in-process Interaction MCP server (internal/interaction) in its
// SESSION + AUTH model: external clients share ONE auth token (a shared secret) but each
// connection needs ISOLATED activation state, so the session id is MINTED on initialize
// (not derived from the token) and auth is a SEPARATE bearer check on every request.
// The security default is loopback-only; exposing to a network REQUIRES a token
// (enforced by the caller mounting this handler).
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProtocolVersion is the MCP revision this server speaks (clients downgrade to it).
const ProtocolVersion = "2025-06-18"

// ToolSpec / CallResult mirror the interaction package shapes (kept local so the two
// servers stay independent).
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type CallResult struct {
	Text    string
	IsError bool
}

// Backend supplies the per-session tool surface and dispatches calls. Session state
// (which backend servers are activated) lives in the Backend, keyed by the minted
// session id the server passes in.
type Backend interface {
	// OpenSession binds a freshly minted session to a workspace (from the initialize
	// request's X-Workspace-Id header; "" means the default workspace). Lets one gateway
	// serve multiple workspaces — each client session sees its own workspace's servers.
	OpenSession(sessionID, workspaceID string)
	// Tools returns the tools advertised for a session: the meta-tools plus any
	// dynamically activated backend-server tools.
	Tools(sessionID string) []ToolSpec
	// Call dispatches a tool call (a meta-tool or a proxied backend tool).
	Call(ctx context.Context, sessionID, name string, args json.RawMessage) (CallResult, error)
	// CloseSession releases any resources a session activated (backend refs), called
	// when the session's stream ends.
	CloseSession(sessionID string)
}

// WorkspaceHeader is the request header an external client sets to choose which
// workspace's MCP servers the gateway exposes. Absent → the default workspace.
const WorkspaceHeader = "X-Workspace-Id"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Server is the streaming gateway MCP HTTP handler.
type Server struct {
	backend Backend
	// auth returns true if the bearer token is accepted. nil → accept all (the mount
	// point must then enforce loopback-only). Checked on every request.
	auth   func(token string) bool
	logger *slog.Logger

	mu      sync.Mutex
	seq     int
	streams map[string]chan []byte // session id -> SSE notify channel
}

// NewServer builds a gateway server. auth may be nil (accept-all; caller enforces
// loopback). logger may be nil.
func NewServer(b Backend, auth func(token string) bool, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{backend: b, auth: auth, logger: logger, streams: map[string]chan []byte{}}
}

func bearer(h string) string {
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

func (s *Server) authorized(r *http.Request) bool {
	if s.auth == nil {
		return true
	}
	return s.auth(bearer(r.Header.Get("Authorization")))
}

func (s *Server) mintSession() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return "gw-" + strconv.Itoa(s.seq)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.serveStream(w, r, r.Header.Get("Mcp-Session-Id"))
	case http.MethodDelete:
		if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
			s.backend.CloseSession(sid)
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		s.servePost(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) servePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeRPC(w, nil, nil, -32700, "read error")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeRPC(w, nil, nil, -32700, "parse error")
		return
	}
	sid := r.Header.Get("Mcp-Session-Id")

	switch req.Method {
	case "initialize":
		sid = s.mintSession()
		s.backend.OpenSession(sid, r.Header.Get(WorkspaceHeader))
		w.Header().Set("Mcp-Session-Id", sid)
		s.writeResult(w, req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"serverInfo":      map[string]string{"name": "tionharness-gateway", "version": "0.0.1"},
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		specs := s.backend.Tools(sid)
		tools := make([]map[string]any, 0, len(specs))
		for _, sp := range specs {
			tools = append(tools, map[string]any{"name": sp.Name, "description": sp.Description, "inputSchema": sp.InputSchema})
		}
		s.writeResult(w, req.ID, map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		res, err := s.backend.Call(r.Context(), sid, p.Name, p.Arguments)
		if err != nil {
			s.logger.Warn("gateway tool call failed", "tool", p.Name, "error", err)
			s.writeResult(w, req.ID, toolContent(err.Error(), true))
			return
		}
		s.writeResult(w, req.ID, toolContent(res.Text, res.IsError))
	default:
		s.writeRPC(w, req.ID, nil, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, sid string) {
	if sid == "" {
		http.Error(w, "missing Mcp-Session-Id", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan []byte, 8)
	s.mu.Lock()
	s.streams[sid] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.streams[sid] == ch {
			delete(s.streams, sid)
		}
		s.mu.Unlock()
		s.backend.CloseSession(sid)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ka := time.NewTicker(20 * time.Second)
	defer ka.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case frame := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", frame)
			flusher.Flush()
		case <-ka.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// PushToolsChanged sends tools/list_changed to a session's open SSE stream. Non-blocking
// no-op when the session has no stream or its buffer is full.
func (s *Server) PushToolsChanged(sessionID string) bool {
	frame, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/tools/list_changed"})
	s.mu.Lock()
	ch := s.streams[sessionID]
	s.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- frame:
		return true
	default:
		s.logger.Warn("gateway list_changed dropped (buffer full)", "session", sessionID)
		return false
	}
}

func toolContent(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isError}
}

func (s *Server) writeResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (s *Server) writeRPC(w http.ResponseWriter, id json.RawMessage, result any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	env := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if code != 0 {
		env["error"] = map[string]any{"code": code, "message": msg}
	} else {
		env["result"] = result
	}
	_ = json.NewEncoder(w).Encode(env)
}
