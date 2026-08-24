// Package interaction implements the TionHarness Interaction MCP server: a minimal
// MCP-over-HTTP (Streamable HTTP) endpoint that lets agent CLIs which run their
// own agentic loop (claude-cli first; Codex/Vibe later) call TionHarness's
// human-in-the-loop tools (ask_user, todo_write, ...) and have them surface in
// the TionHarness UI — the same behaviour the native (anthropic/minimax) tool path
// already provides via a context bridge. See _Docs/11-INTERACTION-MCP.md.
//
// It is also the substrate for the Go-native MCP gateway (Doc 52): the server is
// STATEFUL and STREAMING — it advertises tools.listChanged and holds a per-session
// server->client SSE stream (GET) open so the backend can PUSH
// notifications/tools/list_changed mid-session and grow the advertised tool surface
// on demand (the gateway pattern). Tool-call RESPONSES still return inline on the
// POST; only server-initiated notifications ride the GET stream.
//
// This package is transport+protocol only (no agent/api imports) so it stays
// dependency-light and unit-testable. The caller supplies a Backend that
// resolves a per-run Bearer token to a live turn and dispatches tool calls.
package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
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

// Server implements the Streamable HTTP MCP subset TionHarness needs: initialize,
// notifications/initialized, tools/list, tools/call (POST) plus a long-lived
// server->client SSE stream (GET) carrying tools/list_changed notifications.
type Server struct {
	backend Backend
	logger  *slog.Logger

	mu sync.Mutex
	// streams maps a per-connection key (token + tier) to the channel feeding that
	// connection's open GET SSE stream. A single CLI process opens the SAME token on
	// BOTH the core and extended MCP server entries (two connections, one Bearer), so
	// keying by token alone would let one overwrite the other and a push could land on
	// the wrong connection — the client would never re-list the tier that actually grew.
	// PushToolsChanged broadcasts to every stream for a token so the right tier re-lists
	// (re-listing the other tier is a cheap no-op).
	streams map[string]chan []byte
	// relistWaiters holds one-shot signals waiting for the client's NEXT tools/list on
	// a given stream key (token+tier). The tools/list handler signals them, so
	// PushToolsChangedAndWait can block an activate until the CLI has actually re-listed
	// the grown tier — closing the activate→call race. A live probe (probe_relist_test)
	// confirmed claude-cli 2.1.x re-fetches tools/list concurrently (~10-16ms) while the
	// activate tools/call is still pending, so this wait returns fast and never deadlocks.
	relistWaiters map[string][]chan struct{}
}

// streamKey composes the per-connection stream key from a session token and its tier
// ("core" | "extended" | ""). Two claude MCP connections share the token but differ in
// tier, so this keeps their SSE streams distinct.
func streamKey(token, tier string) string { return token + "\x00" + tier }

// NewServer builds a stateful streaming Interaction MCP server.
func NewServer(b Backend, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{backend: b, logger: logger, streams: map[string]chan []byte{}, relistWaiters: map[string][]chan struct{}{}}
}

// Handler returns the server as an http.Handler (compat shim for existing callers).
func Handler(b Backend, logger *slog.Logger) http.Handler {
	return NewServer(b, logger)
}

func (h *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := bearer(r.Header.Get("Authorization"))
	if token == "" || !h.backend.Valid(token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// GET is the server->client SSE stream: the gateway push channel for
	// notifications/tools/list_changed. Held open for the life of the client
	// connection (one persistent CLI process). Tool-call responses do NOT ride it —
	// they return inline on the POST below.
	if r.Method == http.MethodGet {
		h.serveStream(w, r, token, requestTier(r))
		return
	}
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
			"serverInfo":      map[string]string{"name": "tionharness-interaction", "version": "0.0.1"},
			// Advertise tools.listChanged so the client watches the GET stream and
			// re-fetches tools/list when the surface grows (gateway pattern, Doc 52).
			"capabilities": map[string]any{"tools": map[string]any{"listChanged": true}},
		}, nil)
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		tier := requestTier(r)
		specs := h.backend.Tools(token, tier)
		tools := make([]map[string]any, 0, len(specs))
		for _, s := range specs {
			tools = append(tools, map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"inputSchema": s.InputSchema,
			})
		}
		h.writeRPC(w, req.ID, map[string]any{"tools": tools}, nil)
		// Wake any activate blocked in PushToolsChangedAndWait for this tier: the
		// client has now re-listed, so the grown tool is in its registry and the
		// pending activate can return safely.
		h.signalRelist(streamKey(token, tier))
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

// serveStream holds the server->client SSE stream open for one connection (token+tier)
// and flushes any notification frames pushed for it (the gateway push channel).
func (h *Server) serveStream(w http.ResponseWriter, r *http.Request, token, tier string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	key := streamKey(token, tier)
	ch := h.registerStream(key)
	defer h.unregisterStream(key, ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
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
			// SSE comment line keeps intermediaries from closing an idle stream.
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// registerStream installs (replacing any prior) the notify channel for a per-connection
// key and returns it. A replaced stream's reader exits when its request context ends.
func (h *Server) registerStream(key string) chan []byte {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	h.streams[key] = ch
	h.mu.Unlock()
	h.logger.Debug("interaction stream opened", "key", key)
	return ch
}

// unregisterStream removes the key's stream only if it is still ch (a newer stream may
// have replaced it), so a stale close does not orphan the live channel.
func (h *Server) unregisterStream(key string, ch chan []byte) {
	h.mu.Lock()
	if h.streams[key] == ch {
		delete(h.streams, key)
	}
	h.mu.Unlock()
	h.logger.Debug("interaction stream closed", "key", key)
}

// PushToolsChanged broadcasts a notifications/tools/list_changed frame to EVERY open
// SSE stream for a token (both the core and extended connections), so whichever tier
// grew gets re-listed by the client. Returns true if at least one stream received it.
// No-op (per stream) when a buffer is full; a slow/absent reader never stalls the
// caller (the activate path) — the safety-net TTL and next tools/list still converge.
func (h *Server) PushToolsChanged(token string) bool {
	frame, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "notifications/tools/list_changed",
	})
	prefix := token + "\x00"
	h.mu.Lock()
	var targets []chan []byte
	for key, ch := range h.streams {
		if strings.HasPrefix(key, prefix) {
			targets = append(targets, ch)
		}
	}
	h.mu.Unlock()
	sent := false
	for _, ch := range targets {
		select {
		case ch <- frame:
			sent = true
		default:
			h.logger.Warn("interaction list_changed dropped (stream buffer full)", "session", token)
		}
	}
	if sent {
		h.logger.Debug("interaction pushed tools/list_changed", "session", token, "streams", len(targets))
	}
	return sent
}

// PushToolsChangedAndWait pushes tools/list_changed (like PushToolsChanged) then
// blocks until the client re-fetches tools/list for the EXTENDED tier — confirming
// the just-activated tools are in the CLI's registry — or until timeout. This closes
// the activate→call race: without it, activate returns immediately and a same-turn
// tool call can beat the CLI's re-list and hit "No such tool available: mcp__…".
//
// Safe by construction: it only waits when a push actually reached an open stream
// (no stream → return immediately, nothing would ever signal), and always honors the
// timeout, so a client that failed to re-list can never wedge the activate — worst
// case is +timeout, then the historical behavior (the model retries). The live probe
// (probe_relist_test) measured claude-cli 2.1.x re-listing concurrently in ~10-16ms
// while activate is pending, so in practice this returns almost immediately. Returns
// true if a re-list was observed before the timeout.
func (h *Server) PushToolsChangedAndWait(token string, timeout time.Duration) bool {
	key := streamKey(token, "extended")
	wait := make(chan struct{}, 1)
	h.mu.Lock()
	h.relistWaiters[key] = append(h.relistWaiters[key], wait)
	h.mu.Unlock()

	if !h.PushToolsChanged(token) {
		h.removeRelistWaiter(key, wait) // no open stream → nothing will signal; don't block
		return false
	}
	select {
	case <-wait:
		return true
	case <-time.After(timeout):
		h.removeRelistWaiter(key, wait)
		return false
	}
}

// signalRelist wakes (and clears) every activate waiting on the client's tools/list
// for this stream key. Called by the tools/list handler once it has served the
// re-list, so a pending PushToolsChangedAndWait returns knowing the grown tool is now
// in the client's registry.
func (h *Server) signalRelist(key string) {
	h.mu.Lock()
	waiters := h.relistWaiters[key]
	delete(h.relistWaiters, key)
	h.mu.Unlock()
	for _, w := range waiters {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

// removeRelistWaiter drops a single waiter (timeout / no-stream cleanup) so a
// stale channel is never signalled by a later re-list.
func (h *Server) removeRelistWaiter(key string, wait chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ws := h.relistWaiters[key]
	for i, w := range ws {
		if w == wait {
			h.relistWaiters[key] = append(ws[:i], ws[i+1:]...)
			break
		}
	}
	if len(h.relistWaiters[key]) == 0 {
		delete(h.relistWaiters, key)
	}
}

// HasStream reports whether a session currently has any open SSE stream (a live CLI
// process listening for notifications). Used by the activate path to decide whether a
// mid-turn push can reach the client or must fall back to next-turn re-list.
func (h *Server) HasStream(token string) bool {
	prefix := token + "\x00"
	h.mu.Lock()
	defer h.mu.Unlock()
	for key := range h.streams {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// toolContent builds the MCP tools/call result envelope.
func toolContent(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func (h *Server) writeRPC(w http.ResponseWriter, id json.RawMessage, result any, e *rpcError) {
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

// fullTierQueryParam is the query-string signal a client can add to its extended
// (or core) server URL to request the FULL tier variant ("core-full" /
// "extended-full") instead of the gated one. It rides the query string rather than
// the path so tierFromPath (path.Base) is untouched and every existing "/core" /
// "/extended" URL — including the claude-cli path and its tests — keeps meaning
// exactly what it always meant. Used by codex-cli (see codexMCPSpec/interactionServers
// in internal/agent), whose client never watches tools/list_changed, so the lazy
// activation gate (activate_tools et al.) can never open for it — see
// _Docs/69-CODEX-CLI-SAGLAYICI.md.
const fullTierQueryParam = "full"

// requestTier resolves the wire tier for a request: the path segment
// (core/extended/""), promoted to "<tier>-full" when the request carries
// ?full=1. A bare "" path (no tier segment) ignores the flag — there is no
// "-full" variant of the unscoped/legacy tier.
func requestTier(r *http.Request) string {
	tier := tierFromPath(r.URL.Path)
	if tier == "" {
		return tier
	}
	if r.URL.Query().Get(fullTierQueryParam) != "" {
		return tier + "-full"
	}
	return tier
}

// bearer extracts the token from an "Authorization: Bearer <token>" header.
func bearer(h string) string {
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}
