package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeHTTPServer is a minimal Streamable HTTP MCP server for tests. It answers
// initialize/tools/list over plain JSON and tools/call over an SSE stream, and
// echoes an assigned Mcp-Session-Id to verify the client round-trips it.
type fakeHTTPServer struct {
	sessionID  string
	sawSession string // session id seen on the most recent non-initialize request
	sseForCall bool   // when true, tools/call is answered as an SSE stream
}

func (f *fakeHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req struct {
		ID     *int   `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Method != "initialize" {
		f.sawSession = r.Header.Get("Mcp-Session-Id")
	}

	// Notifications (no id) get a bare 202.
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	writeJSON := func(result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}

	switch req.Method {
	case "initialize":
		if f.sessionID != "" {
			w.Header().Set("Mcp-Session-Id", f.sessionID)
		}
		writeJSON(map[string]any{"protocolVersion": protocolVersion})
	case "tools/list":
		writeJSON(map[string]any{"tools": []map[string]any{
			{"name": "echo", "description": "echo", "inputSchema": map[string]any{"type": "object"}},
		}})
	case "tools/call":
		result := map[string]any{"content": []map[string]any{{"type": "text", "text": "ok:" + req.Params.Name}}}
		if !f.sseForCall {
			writeJSON(result)
			return
		}
		// Answer as an SSE stream: a stray notification first, then the response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", `{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`)
		resp, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
		fmt.Fprintf(w, "data: %s\n\n", resp)
	default:
		writeJSON(map[string]any{})
	}
}

func TestHTTPClientJSONResponse(t *testing.T) {
	srv := httptest.NewServer(&fakeHTTPServer{sessionID: "sess-123"})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialHTTP(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	defer c.Close()

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}
	if c.sessionID != "sess-123" {
		t.Fatalf("session id not captured from initialize: %q", c.sessionID)
	}

	res, err := c.CallTool(ctx, "echo", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Text != "ok:echo" || res.IsError {
		t.Fatalf("call result = %+v", res)
	}
}

func TestHTTPClientSSEResponseAndSessionEcho(t *testing.T) {
	fake := &fakeHTTPServer{sessionID: "sess-xyz", sseForCall: true}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	changed := make(chan struct{}, 1)
	c, err := DialHTTP(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	defer c.Close()
	c.SetOnToolsChanged(func() { changed <- struct{}{} })

	res, err := c.CallTool(ctx, "echo", nil)
	if err != nil {
		t.Fatalf("CallTool over SSE: %v", err)
	}
	if res.Text != "ok:echo" {
		t.Fatalf("SSE call result = %+v", res)
	}
	// The post-initialize request must echo the assigned session id.
	if fake.sawSession != "sess-xyz" {
		t.Fatalf("client did not echo session id; saw %q", fake.sawSession)
	}
	// The in-stream notification must have fired the callback.
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("tools/list_changed callback did not fire from SSE stream")
	}
}

func TestHTTPClientErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := DialHTTP(ctx, srv.URL, nil); err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("want status 500 error, got %v", err)
	}
}

func TestHTTPDialRequiresURL(t *testing.T) {
	if _, err := DialHTTP(context.Background(), "  ", nil); err == nil {
		t.Fatal("want error for empty url")
	}
}
