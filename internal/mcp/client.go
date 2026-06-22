// Package mcp implements a minimal Model Context Protocol client. It speaks
// JSON-RPC 2.0 over a stdio transport (the server is launched as a subprocess)
// and exposes the two calls SwarmGo needs: tools/list and tools/call.
//
// The protocol is intentionally implemented by hand (no SDK) to stay
// dependency-light and match the rest of the codebase.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/bilal-arikan/swarmgo/internal/proc"
	"sync"
	"time"
)

const (
	protocolVersion = "2024-11-05"
	clientName      = "swarmgo"
	clientVersion   = "0.0.1"
)

// Tool describes a tool advertised by an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// rpcRequest / rpcResponse mirror the JSON-RPC 2.0 envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message) }

// StdioClient is a connected stdio MCP server subprocess.
type StdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	mu     sync.Mutex
	nextID int
}

// DialStdio launches the given command as an MCP server and performs the
// initialize handshake. Caller must Close the returned client.
func DialStdio(ctx context.Context, command string, args, env []string) (*StdioClient, error) {
	cmd := exec.Command(command, args...)
	proc.Hide(cmd) // no console flash under the windowless desktop app
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Discard stderr so a chatty server can't block on a full pipe.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %q: %w", command, err)
	}

	c := &StdioClient{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReaderSize(stdout, 1<<20),
		nextID: 1,
	}

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// initialize performs the MCP handshake: initialize request + initialized note.
func (c *StdioClient) initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	// Fire-and-forget the initialized notification (no id, no response).
	return c.notify("notifications/initialized", map[string]any{})
}

// ListTools returns the tools advertised by the server.
func (c *StdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp tools/list decode: %w", err)
	}
	return out.Tools, nil
}

// CallToolResult is the textual result of a tools/call.
type CallToolResult struct {
	Text    string
	IsError bool
}

// CallTool invokes a tool with JSON arguments and flattens the text content.
func (c *StdioClient) CallTool(ctx context.Context, name string, args json.RawMessage) (CallToolResult, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	} else {
		params["arguments"] = map[string]any{}
	}
	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return CallToolResult{}, err
	}
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

// call sends a request and waits for the matching response.
func (c *StdioClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	if err := c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}

	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		for {
			var resp rpcResponse
			line, err := c.reader.ReadBytes('\n')
			if err != nil {
				done <- result{err: fmt.Errorf("mcp read: %w", err)}
				return
			}
			if len(line) == 0 {
				continue
			}
			if err := json.Unmarshal(line, &resp); err != nil {
				continue // skip non-JSON / log lines
			}
			if resp.ID == nil || *resp.ID != id {
				continue // notification or another request's response
			}
			if resp.Error != nil {
				done <- result{err: resp.Error}
				return
			}
			done <- result{raw: resp.Result}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.raw, r.err
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
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.stdin.Write(b)
	return err
}

// Close terminates the server subprocess.
func (c *StdioClient) Close() error {
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
