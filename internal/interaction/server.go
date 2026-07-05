// Package interaction implements the TionSwarm Interaction MCP server: a minimal
// MCP-over-HTTP (Streamable HTTP) endpoint that lets agent CLIs which run their
// own agentic loop (claude-cli first; Codex/Vibe later) call TionSwarm's
// human-in-the-loop tools (ask_user, todo_write, ...) and have them surface in
// the TionSwarm UI — the same behaviour the native (anthropic/minimax) tool path
// already provides via a context bridge. See _Docs/11-INTERACTION-MCP.md.
//
// This package is transport+protocol only (no agent/api imports) so it stays
// dependency-light and unit-testable. The caller supplies a Backend that
// resolves a per-run Bearer token to a live turn and dispatches tool calls.
package interaction

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// ProtocolVersion is the MCP protocol revision this server speaks. Clients may
// propose a newer one (claude-code proposes 2025-11-25); the handshake returns
// this value and clients downgrade to it (verified in the Faz 0 spike).
const ProtocolVersion = "2025-06-18"

// ToolSpec is one tool advertised in tools/list. InputSchema is a raw JSON
// Schema object (sourced from the single tool definition, never re-declared).
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// CallResult is the outcome of a tools/call. IsError flags a tool-level failure
// (returned to the model as content, not a transport error) so the model can
// recover on its own.
type CallResult struct {
	Text    string
	IsError bool
}

// Backend resolves Bearer tokens to live turns and dispatches tool calls. The
// api layer implements it over the in-flight chat-run registry.
type Backend interface {
	// Valid reports whether token maps to a live run.
	Valid(token string) bool
	// Tools returns the tool set advertised for the run identified by token: the
	// static interaction tools plus any per-run bridged tools (e.g. the CLI
	// self-management catalog). token is always valid here (checked before call).
	// tier ("core" | "extended" | "") selects the advertised subset so the CLI can
	// wire each tier to its own MCP server entry (alwaysLoad core vs deferred
	// extended); "" returns the full set (legacy / single-endpoint callers).
	Tools(token, tier string) []ToolSpec
	// Call dispatches a tool call for the run identified by token. A blocking
	// tool (ask_user) returns once the user answers, ctx is cancelled, or the
	// turn ends.
	Call(ctx context.Context, token, name string, args json.RawMessage) (CallResult, error)
}

// --- JSON-RPC 2.0 envelope (local; the mcp client package keeps its own) ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // number | string | absent (notification)
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handler returns an http.Handler implementing the Streamable HTTP MCP subset
// TionSwarm needs: initialize, notifications/initialized, tools/list, tools/call.
func Handler(b Backend, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &mcpHandler{backend: b, logger: logger}
}

type mcpHandler struct {
	backend Backend
	logger  *slog.Logger
}

func (h *mcpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := bearer(r.Header.Get("Authorization"))
	if token == "" || !h.backend.Valid(token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// GET is the optional server->client SSE stream. The MVP returns tool
	// results inline on the POST response, so we don't open one (claude tolerates
	// a 405 here — verified in the spike).
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeRPC(w, nil, nil, &rpcError{Code: -32700, Message: "read error"})
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeRPC(w, nil, nil, &rpcError{Code: -32700, Message: "parse error"})
		return
	}

	switch req.Method {
	case "initialize":
		// Bind the MCP session to the run token so the spec's session-id round-trip
		// has a stable value (we key everything off the Bearer token regardless).
		w.Header().Set("Mcp-Session-Id", token)
		h.writeRPC(w, req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"serverInfo":      map[string]string{"name": "tionswarm-interaction", "version": "0.0.1"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}, nil)
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		specs := h.backend.Tools(token, tierFromPath(r.URL.Path))
		tools := make([]map[string]any, 0, len(specs))
		for _, s := range specs {
			tools = append(tools, map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"inputSchema": s.InputSchema,
			})
		}
		h.writeRPC(w, req.ID, map[string]any{"tools": tools}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		res, err := h.backend.Call(r.Context(), token, p.Name, p.Arguments)
		if err != nil {
			// Surface as a tool-level error so the model can proceed rather than
			// aborting the whole CLI loop.
			h.logger.Warn("interaction tool call failed", "tool", p.Name, "error", err)
			h.writeRPC(w, req.ID, toolContent(err.Error(), true), nil)
			return
		}
		h.writeRPC(w, req.ID, toolContent(res.Text, res.IsError), nil)
	default:
		h.writeRPC(w, req.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method})
	}
}

// toolContent builds the MCP tools/call result envelope.
func toolContent(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func (h *mcpHandler) writeRPC(w http.ResponseWriter, id json.RawMessage, result any, e *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: e}
	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

// tierFromPath derives the advertised tier from the request path's last segment
// so two CLI MCP-config entries (.../core, .../extended) pointing at the same
// in-process handler get different tool subsets. Any other segment (e.g. the bare
// /mcp/interaction mount) yields "" → the full set. Decoupled from the mount path.
func tierFromPath(p string) string {
	switch path.Base(p) {
	case "core":
		return "core"
	case "extended":
		return "extended"
	default:
		return ""
	}
}

// bearer extracts the token from an "Authorization: Bearer <token>" header.
func bearer(h string) string {
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}
