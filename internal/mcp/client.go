// Package mcp implements a minimal Model Context Protocol client. It speaks
// JSON-RPC 2.0 over a stdio transport (the server is launched as a subprocess)
// and exposes the calls TionHarness needs: tools/list and tools/call.
//
// The protocol is intentionally implemented by hand (no SDK) to stay
// dependency-light and match the rest of the codebase.
//
// The client is safe for a PERSISTENT, concurrently-used connection (see Pool):
// a single background read loop demultiplexes responses to per-call channels by
// JSON-RPC id, and server-initiated notifications (notably
// notifications/tools/list_changed) are delivered to an optional callback.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

const (
	protocolVersion = "2024-11-05"
	clientName      = "tionharness"
	clientVersion   = "0.0.1"
)

// Client is the transport-agnostic surface the pool and catalog builders use. It
// is satisfied by StdioClient (subprocess transport) and httpClient (Streamable
// HTTP transport). Keeping callers on this interface lets a server's transport be
// chosen at dial time without the pool knowing which one it got.
type Client interface {
	// ListTools returns the tools the server currently advertises.
	ListTools(ctx context.Context) ([]Tool, error)
	// CallTool invokes a tool with JSON arguments and flattens the text content.
	CallTool(ctx context.Context, name string, args json.RawMessage) (CallToolResult, error)
	// Alive reports whether the connection is still usable.
	Alive() bool
	// Close terminates the connection (and any subprocess).
	Close() error
	// SetOnToolsChanged registers a callback fired when the server announces a
	// tools/list change. Transports without a server→client channel may never
	// fire it; that is fine — the pool's TTL still refreshes.
	SetOnToolsChanged(fn func())
	// SetLogger attaches an optional logger (and owning server name) for
	// connection-lifecycle / decode-noise diagnostics. Nil-safe.
	SetLogger(l *slog.Logger, server string)
}

// Compile-time assertions that both transports satisfy Client.
var (
	_ Client = (*StdioClient)(nil)
	_ Client = (*httpClient)(nil)
)

// Tool describes a tool advertised by an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// rpcRequest mirrors the JSON-RPC 2.0 request/notification envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcMessage is a permissive inbound envelope: it is a response when ID is set
// (Result/Error populated) and a server notification when Method is set.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message) }

// reply is what a pending call receives from the read loop.
type reply struct {
	result json.RawMessage
	err    error
}

// StdioClient is a connected stdio MCP server subprocess. Obtain one from
// DialStdio. It is safe for concurrent use.
type StdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	writeMu sync.Mutex // serializes writes to stdin

	mu       sync.Mutex
	nextID   int
	pending  map[int]chan reply
	closed   bool
	onChange func() // invoked (async) on notifications/tools/list_changed

	logger     *slog.Logger // optional: read-loop death / decode noise (nil-safe)
	serverName string       // server label attached to logs (set with the logger)
}

// SetLogger attaches a logger (and the owning server's name for context) so the
// read loop can surface why a connection died and how much inbound noise it
// skipped. Optional and nil-safe; the Pool wires this after dialing. The stdio
// client is otherwise opaque — these logs reach the in-app Logs screen.
func (c *StdioClient) SetLogger(l *slog.Logger, server string) {
	c.mu.Lock()
	c.logger = l
	c.serverName = server
	c.mu.Unlock()
}

// log emits at the given level via the attached logger, if any. Nil-safe and
// reads the logger under the lock so SetLogger races are harmless.
func (c *StdioClient) log(level slog.Level, msg string, args ...any) {
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

// DialStdio launches the given command as an MCP server and performs the
// initialize handshake. Caller must Close the returned client. A non-empty dir
// sets the subprocess working directory (empty inherits the host cwd) — used to
// place a file-writing server's allowed root at the caller's scratchpad.
func DialStdio(ctx context.Context, command string, args, env []string, dir string) (*StdioClient, error) {
	cmd := proc.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Keep the tail of stderr instead of discarding it. A chatty server still
	// cannot block, because stderrTail never blocks and never grows -- but when the
	// handshake fails, the server's own explanation is available instead of a bare
	// "EOF" (see stderrtail.go for the incident that motivated this).
	tail := &stderrTail{}
	cmd.Stderr = tail

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %q: %w", command, err)
	}

	c := &StdioClient{
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewReaderSize(stdout, 1<<20),
		nextID:  1,
		pending: map[int]chan reply{},
	}
	go c.readLoop()

	if err := c.initialize(ctx); err != nil {
		// Close first: it stops stdin and waits for the process, which is what lets
		// os/exec finish copying stderr. Close caps that wait and then kills, so back
		// it up with a short bounded wait -- otherwise the one diagnostic we came for
		// can be missed on a loaded machine.
		_ = c.Close()
		tail.waitNonEmpty(2 * time.Second)
		if msg := tail.Tail(); msg != "" {
			return nil, fmt.Errorf("%w (server stderr: %s)", err, msg)
		}
		return nil, err
	}
	return c, nil
}

// SetOnToolsChanged registers a callback invoked (in its own goroutine) whenever
// the server announces notifications/tools/list_changed.
func (c *StdioClient) SetOnToolsChanged(fn func()) {
	c.mu.Lock()
	c.onChange = fn
	c.mu.Unlock()
}

// Alive reports whether the connection is still usable (read loop running and
// not closed).
func (c *StdioClient) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

// readLoop is the single reader: it demultiplexes responses by id and routes
// notifications. It exits when the pipe closes (or Close kills the process),
// failing every in-flight call so no caller blocks forever.
func (c *StdioClient) readLoop() {
	for {
		line, err := c.reader.ReadBytes('\n')
		if len(line) > 0 {
			c.dispatch(line)
		}
		if err != nil {
			c.failAll(err)
			return
		}
	}
}

// dispatch parses one inbound line and either delivers a response to its waiting
// call or handles a server notification. Non-JSON / log lines are ignored.
func (c *StdioClient) dispatch(line []byte) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.log(slog.LevelDebug, "mcp client: skipped non-JSON inbound line", "bytes", len(line))
		return // skip non-JSON / log noise
	}
	if msg.ID != nil {
		c.mu.Lock()
		ch := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if ch == nil {
			c.log(slog.LevelDebug, "mcp client: response for unknown/late id", "id", *msg.ID)
			return // late/unknown id
		}
		if msg.Error != nil {
			ch <- reply{err: msg.Error}
		} else {
			ch <- reply{result: msg.Result}
		}
		return
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
}

// failAll marks the client closed and delivers err to every pending call.
func (c *StdioClient) failAll(err error) {
	c.mu.Lock()
	wasClosed := c.closed // distinguish our own Close() from an unexpected death
	c.closed = true
	pend := c.pending
	c.pending = map[int]chan reply{}
	c.mu.Unlock()
	// A read-loop exit we did NOT initiate means the server process died or its
	// pipe broke mid-session — surface why, and how many calls were stranded.
	// A clean EOF after our own Close() is routine teardown, so stay quiet.
	if !wasClosed {
		level := slog.LevelWarn
		if errors.Is(err, io.EOF) {
			level = slog.LevelInfo
		}
		c.log(level, "mcp client: read loop exited (connection lost)", "error", err.Error(), "pending", len(pend))
	}
	for _, ch := range pend {
		ch <- reply{err: fmt.Errorf("mcp read: %w", err)}
	}
}

// initialize performs the MCP handshake: initialize request + initialized note.
// It advertises tools.listChanged so servers send incremental updates we honor
// via the onChange callback.
func (c *StdioClient) initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
		"clientInfo":      map[string]string{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	// Fire-and-forget the initialized notification (no id, no response).
	return c.notify("notifications/initialized", map[string]any{})
}

// CallToolResult is the textual result of a tools/call.
type CallToolResult struct {
	Text    string
	IsError bool
}

// callToolParams builds the tools/call request params, defaulting empty args to
// an empty object (servers reject a missing "arguments"). Shared by transports.
func callToolParams(name string, args json.RawMessage) map[string]any {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	} else {
		params["arguments"] = map[string]any{}
	}
	return params
}

// parseToolsList decodes a tools/list result envelope. Shared by transports.
func parseToolsList(raw json.RawMessage) ([]Tool, error) {
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp tools/list decode: %w", err)
	}
	return out.Tools, nil
}

// parseCallResult decodes a tools/call result and flattens its text content.
// Shared by transports.
func parseCallResult(raw json.RawMessage) (CallToolResult, error) {
	var out struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return CallToolResult{}, fmt.Errorf("mcp tools/call decode: %w", err)
	}
	var text string
	for _, b := range out.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	return CallToolResult{Text: text, IsError: out.IsError}, nil
}

// ListTools returns the tools advertised by the server.
func (c *StdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseToolsList(raw)
}

// CallTool invokes a tool with JSON arguments and flattens the text content.
func (c *StdioClient) CallTool(ctx context.Context, name string, args json.RawMessage) (CallToolResult, error) {
	raw, err := c.call(ctx, "tools/call", callToolParams(name, args))
	if err != nil {
		return CallToolResult{}, err
	}
	return parseCallResult(raw)
}

// call sends a request and waits for the matching response (routed by the read
// loop). It returns promptly on ctx cancellation or connection death.
func (c *StdioClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("mcp: connection closed")
	}
	id := c.nextID
	c.nextID++
	ch := make(chan reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case r := <-ch:
		return r.result, r.err
	}
}

func (c *StdioClient) notify(method string, params any) error {
	return c.write(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *StdioClient) write(req rpcRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(b)
	return err
}

// Close terminates the server subprocess. Pending calls unblock with an error
// once the read loop observes the closed pipe.
func (c *StdioClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		// Give it a moment, then kill.
		done := make(chan struct{})
		go func() { _ = c.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}
	return nil
}
