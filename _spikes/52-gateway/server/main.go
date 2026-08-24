// Spike MCP server for _Docs/52 gateway plan — Q1/Q2/Q3 validation.
//
// A minimal STATEFUL streamable-HTTP MCP server that mirrors what the real
// TionHarness Interaction MCP server must become (Faz 1): it holds a per-session
// server->client SSE stream open (GET) and can PUSH notifications/tools/list_changed
// mid-turn. It starts by advertising a small tool surface (spike_ping, spike_grow)
// and only registers spike_secret AFTER spike_grow is called — pushing list_changed
// so the client re-lists and (Q1) can call the newly-appeared tool in the SAME turn.
//
// Wire subset implemented (MCP 2025-06-18 streamable HTTP):
//   POST /mcp  -> initialize | notifications/initialized | tools/list | tools/call
//   GET  /mcp  -> long-lived text/event-stream carrying server->client notifications
//
// Run: go run . -addr 127.0.0.1:8791   (stderr logs every RPC for the harness)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const protocolVersion = "2025-06-18"

// session holds per-connection state: whether spike_grow has run (which unlocks
// spike_secret) and the channel feeding this session's open SSE GET stream.
type session struct {
	mu     sync.Mutex
	grown  bool
	notify chan []byte // raw JSON-RPC notification frames to flush on the SSE stream
}

type server struct {
	mu       sync.Mutex
	sessions map[string]*session
	seq      int
}

func newServer() *server { return &server{sessions: map[string]*session{}} }

func (s *server) newSession() (string, *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("spike-%d", s.seq)
	sess := &session{notify: make(chan []byte, 8)}
	s.sessions[id] = sess
	return id, sess
}

func (s *server) getSession(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// toolList returns the advertised tool set for a session. spike_secret only
// appears once the session has "grown" — this is the dynamic surface under test.
func toolList(grown bool) []map[string]any {
	tools := []map[string]any{
		{
			"name":        "spike_ping",
			"description": "Health probe. Returns 'pong'.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "spike_grow",
			"description": "Unlocks a hidden tool (spike_secret) and notifies the client via tools/list_changed. Call this first.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
	if grown {
		tools = append(tools, map[string]any{
			"name":        "spike_secret",
			"description": "Returns the secret token. Only available AFTER spike_grow.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}
	return tools
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func writeJSON(w http.ResponseWriter, sessionID string, id json.RawMessage, result any) {
	if sessionID != "" {
		w.Header().Set("Mcp-Session-Id", sessionID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result,
	})
}

func (s *server) handlePost(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req rpcReq
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}
	sessionID := r.Header.Get("Mcp-Session-Id")
	log.Printf("POST method=%s session=%s", req.Method, sessionID)

	switch req.Method {
	case "initialize":
		id, _ := s.newSession()
		writeJSON(w, id, req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"serverInfo":      map[string]string{"name": "spike-gateway", "version": "0.0.1"},
			// Advertise listChanged so the client keeps an eye on the SSE stream.
			"capabilities": map[string]any{"tools": map[string]any{"listChanged": true}},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		sess := s.getSession(sessionID)
		grown := sess != nil && func() bool { sess.mu.Lock(); defer sess.mu.Unlock(); return sess.grown }()
		writeJSON(w, sessionID, req.ID, map[string]any{"tools": toolList(grown)})
	case "tools/call":
		var p struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(req.Params, &p)
		s.handleCall(w, sessionID, req.ID, p.Name)
	default:
		writeJSON(w, sessionID, req.ID, map[string]any{})
	}
}

func (s *server) handleCall(w http.ResponseWriter, sessionID string, id json.RawMessage, name string) {
	sess := s.getSession(sessionID)
	switch name {
	case "spike_ping":
		writeJSON(w, sessionID, id, textResult("pong"))
	case "spike_grow":
		if sess != nil {
			sess.mu.Lock()
			sess.grown = true
			sess.mu.Unlock()
			// Push list_changed on the session's SSE stream BEFORE returning the call
			// result, so by the time the client reads "grown" the notification is
			// already queued (YENI-D ordering mitigation under test).
			frame, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "method": "notifications/tools/list_changed",
			})
			select {
			case sess.notify <- frame:
				log.Printf("pushed list_changed to session=%s", sessionID)
			default:
				log.Printf("WARN notify channel full for session=%s", sessionID)
			}
		}
		writeJSON(w, sessionID, id, textResult("grown; spike_secret is now available — call it now"))
	case "spike_secret":
		if sess == nil || !func() bool { sess.mu.Lock(); defer sess.mu.Unlock(); return sess.grown }() {
			writeJSON(w, sessionID, id, errResult("spike_secret is locked; call spike_grow first"))
			return
		}
		writeJSON(w, sessionID, id, textResult("SECRET=GATEWAY_OK_42"))
	default:
		writeJSON(w, sessionID, id, errResult("unknown tool: "+name))
	}
}

func textResult(s string) map[string]any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": s}}, "isError": false}
}
func errResult(s string) map[string]any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": s}}, "isError": true}
}

// handleGet holds the server->client SSE stream open and flushes any notification
// frames queued for the session (the gateway push channel).
func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	sess := s.getSession(sessionID)
	log.Printf("GET (SSE open) session=%s found=%v", sessionID, sess != nil)
	if sess == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flusher", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ka := time.NewTicker(15 * time.Second)
	defer ka.Stop()
	for {
		select {
		case <-r.Context().Done():
			log.Printf("GET stream closed session=%s", sessionID)
			return
		case frame := <-sess.notify:
			fmt.Fprintf(w, "data: %s\n\n", frame)
			flusher.Flush()
			log.Printf("flushed notification session=%s", sessionID)
		case <-ka.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleGet(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8791", "listen address")
	flag.Parse()
	s := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handle)
	log.Printf("spike gateway MCP listening on http://%s/mcp", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
