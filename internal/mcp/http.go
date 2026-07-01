package mcp

// Streamable HTTP transport (MCP spec revision 2025-03-26 / 2025-06-18).
//
// Unlike the older "HTTP+SSE" transport (two endpoints, deprecated), Streamable
// HTTP uses a SINGLE endpoint:
//
//   - Every client→server message is an HTTP POST whose body is one JSON-RPC
//     message. The client advertises Accept: application/json, text/event-stream.
//   - A POST carrying a REQUEST gets back either a single application/json body
//     (one JSON-RPC response) or a text/event-stream (an SSE stream whose events
//     carry JSON-RPC messages; the response to our request arrives as one of
//     them). A POST carrying only notifications/responses gets back 202 Accepted
//     with no body.
//   - The server MAY assign a session by returning an Mcp-Session-Id header on
//     the initialize response; the client then echoes it on every later request.
//   - The client SHOULD send MCP-Protocol-Version on every request after init.
//   - On close the client SHOULD DELETE the session (best-effort).
//
// Only Streamable HTTP is implemented (the deprecated HTTP+SSE transport is not).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// httpProtocolVersion is advertised via the MCP-Protocol-Version header on
// post-initialize requests. It tracks the Streamable HTTP revision we target.
const httpProtocolVersion = "2025-06-18"

// httpClient is a connected Streamable HTTP MCP server. Obtain one from DialHTTP.
// It is safe for concurrent use: every call is an independent POST, so no
// per-connection demux is needed (HTTP correlates response to request).
type httpClient struct {
	url     string
	hc      *http.Client
	headers map[string]string // static headers (e.g. Authorization), applied to every request

	mu        sync.Mutex
	nextID    int
	sessionID string // Mcp-Session-Id assigned by the server on initialize (may stay "")
	closed    bool
	onChange  func() // invoked (async) on notifications/tools/list_changed

	logger     *slog.Logger
	serverName string
}

// DialHTTP opens a Streamable HTTP MCP connection and performs the initialize
// handshake. Caller must Close the returned client. headers carries any static
// request headers (typically Authorization for a remote server).
func DialHTTP(ctx context.Context, url string, headers map[string]string) (*httpClient, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("mcp: http transport requires a url")
	}
	c := &httpClient{
		url:     url,
		hc:      &http.Client{}, // no client-side timeout: caller's ctx bounds each call
		headers: headers,
		nextID:  1,
	}
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// SetOnToolsChanged registers the tools/list_changed callback. Streamable HTTP
// without a server-opened stream may never deliver one — the pool's TTL covers
// that case — but a server that answers requests over SSE can emit it inline.
func (c *httpClient) SetOnToolsChanged(fn func()) {
	c.mu.Lock()
	c.onChange = fn
	c.mu.Unlock()
}

// SetLogger attaches a logger and the owning server name for diagnostics.
func (c *httpClient) SetLogger(l *slog.Logger, server string) {
	c.mu.Lock()
	c.logger = l
	c.serverName = server
	c.mu.Unlock()
}

func (c *httpClient) log(level slog.Level, msg string, args ...any) {
	c.mu.Lock()
	l, name := c.logger, c.serverName
	c.mu.Unlock()
	if l == nil {
		return
	}
	if name != "" {
		args = append([]any{"server", name}, args...)
	}
	l.Log(context.Background(), level, msg, args...)
}

// Alive reports whether the connection has not been closed. HTTP is connectionless
// between calls, so there is no live socket to probe; a dead endpoint surfaces as
// a per-call error instead.
func (c *httpClient) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

// initialize performs the MCP handshake: initialize request + initialized note.
func (c *httpClient) initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
		"clientInfo":      map[string]string{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	// Fire-and-forget the initialized notification (no id, no response).
	return c.notify(ctx, "notifications/initialized", map[string]any{})
}

// ListTools returns the tools advertised by the server.
func (c *httpClient) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseToolsList(raw)
}

// CallTool invokes a tool with JSON arguments and flattens the text content.
func (c *httpClient) CallTool(ctx context.Context, name string, args json.RawMessage) (CallToolResult, error) {
	raw, err := c.call(ctx, "tools/call", callToolParams(name, args))
	if err != nil {
		return CallToolResult{}, err
	}
	return parseCallResult(raw)
}

// call POSTs a JSON-RPC request and returns the matching response result. The
// server may answer with a single JSON body or an SSE stream; both are handled.
func (c *httpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("mcp: connection closed")
	}
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// initialize may assign a session id we must echo on every later request.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("mcp http %s: status %d: %s", method, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "text/event-stream"):
		return c.readSSEResult(resp.Body, id)
	default:
		// Treat anything else as a single JSON-RPC response body.
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("mcp http %s: read body: %w", method, err)
		}
		return resultFromMessage(data, id)
	}
}

// notify POSTs a JSON-RPC notification (no id, no response expected). A 202/200
// with no body is the normal outcome.
func (c *httpClient) notify(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	resp, err := c.post(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp http notify %s: status %d", method, resp.StatusCode)
	}
	return nil
}

// post issues one POST to the server endpoint with the standard MCP headers and
// the static + session headers applied.
func (c *httpClient) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", httpProtocolVersion)
	c.mu.Lock()
	sid := c.sessionID
	headers := c.headers
	c.mu.Unlock()
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.hc.Do(req)
}

// readSSEResult consumes an SSE stream until it finds the JSON-RPC response whose
// id matches wantID. Server-initiated notifications (e.g. tools/list_changed)
// encountered along the way are dispatched. The stream is read to that response;
// anything after it is ignored.
func (c *httpClient) readSSEResult(body io.Reader, wantID int) (json.RawMessage, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var data strings.Builder
	flush := func() (json.RawMessage, bool, error) {
		if data.Len() == 0 {
			return nil, false, nil
		}
		payload := data.String()
		data.Reset()
		raw, matched, err := c.handleSSEMessage([]byte(payload), wantID)
		return raw, matched, err
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			// Event boundary: process the accumulated data payload.
			if raw, matched, err := flush(); err != nil || matched {
				return raw, err
			}
		case strings.HasPrefix(line, "data:"):
			// Per the SSE grammar, a single optional leading space is stripped.
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		default:
			// Ignore other SSE fields (event:, id:, retry:, comments).
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("mcp http sse: read: %w", err)
	}
	// Stream ended on a final event without a trailing blank line.
	if raw, matched, err := flush(); err != nil || matched {
		return raw, err
	}
	return nil, fmt.Errorf("mcp http sse: stream closed before response to id %d", wantID)
}

// handleSSEMessage parses one SSE data payload as a JSON-RPC message. If it is the
// response to wantID, it returns (result, true, err). If it is a server
// notification, it is dispatched and (nil, false, nil) returned so the caller
// keeps reading.
func (c *httpClient) handleSSEMessage(payload []byte, wantID int) (json.RawMessage, bool, error) {
	var msg rpcMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		c.log(slog.LevelDebug, "mcp http: skipped non-JSON SSE payload", "bytes", len(payload))
		return nil, false, nil
	}
	if msg.ID != nil {
		if *msg.ID != wantID {
			return nil, false, nil // a response to some other request; ignore
		}
		if msg.Error != nil {
			return nil, true, msg.Error
		}
		return msg.Result, true, nil
	}
	// Server-initiated notification.
	if msg.Method == "notifications/tools/list_changed" {
		c.mu.Lock()
		fn := c.onChange
		c.mu.Unlock()
		if fn != nil {
			go fn()
		}
	}
	return nil, false, nil
}

// resultFromMessage decodes a single JSON-RPC response body and returns its
// result, requiring the id to match wantID.
func resultFromMessage(data []byte, wantID int) (json.RawMessage, error) {
	var msg rpcMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &msg); err != nil {
		return nil, fmt.Errorf("mcp http: decode response: %w", err)
	}
	if msg.Error != nil {
		return nil, msg.Error
	}
	if msg.ID == nil || *msg.ID != wantID {
		return nil, fmt.Errorf("mcp http: response id mismatch (want %d)", wantID)
	}
	return msg.Result, nil
}

// Close marks the client closed and best-effort DELETEs the server session.
func (c *httpClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	sid := c.sessionID
	c.mu.Unlock()

	// Best-effort session teardown: a server that assigned a session id MAY accept
	// a DELETE to end it. Ignore the outcome (many servers return 405).
	if sid != "" {
		req, err := http.NewRequest(http.MethodDelete, c.url, nil)
		if err == nil {
			req.Header.Set("Mcp-Session-Id", sid)
			req.Header.Set("MCP-Protocol-Version", httpProtocolVersion)
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			if resp, err := c.hc.Do(req); err == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
				_ = resp.Body.Close()
			}
		}
	}
	return nil
}
