// Package codemode implements the "code execution with MCP" pattern
// (_Docs/44): MCP tools are exposed to the agent as a generated Python module
// tree instead of per-turn tool schemas, and the agent calls them by writing
// code. The two pieces are the per-execution loopback Bridge (this file) that
// dispatches a script's MCP calls into TionSwarm's connection pool, and the
// binding generator (bindings.go) that renders the Python modules.
package codemode

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

const (
	// bridgeMaxCalls caps how many MCP calls one script execution may make — a
	// runaway-loop backstop, far above any legitimate script.
	bridgeMaxCalls = 200
	// bridgeCallTimeout bounds a single dispatched MCP call.
	bridgeCallTimeout = 120 * time.Second
	// bridgeMaxBodyBytes caps a /call request body (tool name + args JSON).
	bridgeMaxBodyBytes = 1 * 1024 * 1024
)

// CallFunc dispatches one namespaced ("server__tool") MCP call. Production
// wires it to the workspace's persistent MCP pool; tests use fakes.
type CallFunc func(ctx context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error)

// GateFunc decides whether one in-script MCP call may run (per-call permission,
// Faz 2 of _Docs/44). It returns (allowed, denialMessage); a denial comes back
// to the script as a loud tool error (the Python binding raises MCPError). The
// caller binds any context it needs (permission mode, prompter, grants) into
// the closure — the bridge itself stays policy-free.
type GateFunc func(tool string, args json.RawMessage) (allowed bool, denial string)

// CallObservation is the per-call record handed to the Observe hook — the
// observability that replaces individual tool_use cards in code mode (each
// dispatched call becomes a debug-journal event AND a nested trace step on the
// agent side). Args is the call input for the UI card — it never re-enters the
// model's context, only the rendered trace.
type CallObservation struct {
	Tool     string          // namespaced tool name
	Args     json.RawMessage // call arguments (for the trace card)
	DurMs    int64           // dispatch latency (0 for denied calls — they never ran)
	OutBytes int             // result text size
	IsError  bool            // tool-level failure (includes denials)
	Denied   bool            // blocked by the permission gate before dispatch
}

// ObserveFunc receives one CallObservation per bridge call (dispatched OR
// denied). Called synchronously from the handler; keep it cheap.
type ObserveFunc func(CallObservation)

// Config wires a Bridge: the MCP dispatcher is required, everything else optional.
type Config struct {
	Call CallFunc // dispatches allowed MCP calls (required)
	// Builtin dispatches an allowed TionSwarm built-in call by its BARE name (the
	// tool arrives namespaced as "tionswarm__<tool>" and is stripped first). Nil ⇒
	// built-in bindings are disabled (a tionswarm__ call comes back as a loud error).
	Builtin CallFunc
	Allow   func(string) bool // agent tool filter (nil = allow all); checked on the BARE name
	Gate    GateFunc          // per-call permission (nil = allow all); checked on the BARE name
	Observe ObserveFunc       // per-call observability (nil = none)
}

// Bridge is a per-execution loopback HTTP endpoint a run_code script calls to
// reach MCP tools. It lives only for the duration of one script run: Start it,
// hand its URL+token to the subprocess, Close it when the script exits. Every
// call is authenticated (per-execution random Bearer token), filtered (allow
// predicate — the same tool filter the agent's native tool loop uses, so code
// mode grants NO capability beyond direct tool calls), permission-gated (Gate —
// the same permGate the native loop runs per tool call) and counted.
type Bridge struct {
	ln  net.Listener
	srv *http.Server
	cfg Config

	token string

	mu     sync.Mutex
	counts map[string]int // namespaced tool -> call count
	total  int
	errs   int
	denied int
}

// Start opens the loopback listener on an OS-assigned port and begins serving.
// cfg.Call must be non-nil; the other hooks may be nil.
func Start(cfg Config) (*Bridge, error) {
	if cfg.Call == nil {
		return nil, fmt.Errorf("codemode: bridge requires a call dispatcher")
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return nil, fmt.Errorf("codemode: token generation failed: %w", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("codemode: bridge listen failed: %w", err)
	}
	b := &Bridge{
		ln:     ln,
		cfg:    cfg,
		token:  hex.EncodeToString(tok),
		counts: map[string]int{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/call", b.handleCall)
	b.srv = &http.Server{Handler: mux}
	go func() { _ = b.srv.Serve(ln) }()
	return b, nil
}

// URL returns the bridge base URL (http://127.0.0.1:<port>).
func (b *Bridge) URL() string { return "http://" + b.ln.Addr().String() }

// Token returns the per-execution Bearer token.
func (b *Bridge) Token() string { return b.token }

// Close stops the listener. Safe to call once the script has exited.
func (b *Bridge) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = b.srv.Shutdown(ctx)
}

// callRequest is the JSON body a binding posts to /call.
type callRequest struct {
	Tool string          `json:"tool"` // namespaced: server__tool
	Args json.RawMessage `json:"args"`
}

// callResponse is the JSON body returned for a dispatched call. Tool-level
// failures come back as isError=true (HTTP 200); protocol failures (bad token,
// disallowed tool, malformed body) use HTTP error codes with {"error": ...}.
type callResponse struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError"`
}

func (b *Bridge) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeBridgeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), []byte(b.token)) != 1 {
		writeBridgeErr(w, http.StatusUnauthorized, "invalid bridge token")
		return
	}
	var req callRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, bridgeMaxBodyBytes))
	if err := dec.Decode(&req); err != nil {
		writeBridgeErr(w, http.StatusBadRequest, "malformed call body: "+err.Error())
		return
	}
	server, bare, ok := mcp.SplitNamespaced(req.Tool)
	if !ok {
		writeBridgeErr(w, http.StatusBadRequest, fmt.Sprintf("%q is not a namespaced tool (server__tool)", req.Tool))
		return
	}
	// Choose the dispatch target. Built-ins live under the reserved BuiltinServer
	// namespace; every other namespace is an MCP server. gateName is the name used
	// for the allow filter, the permission gate and the debug event — the BARE
	// name for built-ins (so it matches the agent's own tool names) and the full
	// namespaced name for MCP.
	dispatch := b.cfg.Call
	dispatchName := req.Tool
	gateName := req.Tool
	if server == BuiltinServer {
		if b.cfg.Builtin == nil {
			writeBridgeErr(w, http.StatusBadRequest, "built-in tools are not available in this run_code execution")
			return
		}
		dispatch = b.cfg.Builtin
		dispatchName = bare
		gateName = bare
	}
	if b.cfg.Allow != nil && !b.cfg.Allow(gateName) {
		writeBridgeErr(w, http.StatusForbidden, fmt.Sprintf("tool %q is not permitted for this agent", gateName))
		return
	}
	if !b.registerCall(req.Tool) {
		writeBridgeErr(w, http.StatusTooManyRequests, fmt.Sprintf("bridge call limit reached (%d per execution)", bridgeMaxCalls))
		return
	}

	// Per-call permission gate (Faz 2): the same decision the native tool loop
	// would make for this call in this turn — "ask" mode prompts the user (the
	// script blocks on the HTTP response meanwhile), standing grants short-
	// circuit, denials come back as loud tool errors the script can react to.
	if b.cfg.Gate != nil {
		if allowed, denial := b.cfg.Gate(gateName, req.Args); !allowed {
			b.registerDenied()
			b.observe(CallObservation{Tool: gateName, Args: req.Args, IsError: true, Denied: true})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(callResponse{Text: denial, IsError: true})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), bridgeCallTimeout)
	defer cancel()
	start := time.Now()
	out, err := dispatch(ctx, dispatchName, req.Args)
	resp := callResponse{Text: out.Text, IsError: out.IsError}
	if err != nil {
		// Dispatcher failures surface as tool errors so the Python side raises a
		// single MCPError type for both — a failed call must fail loudly in the
		// script, never be swallowed.
		resp = callResponse{Text: "mcp call failed: " + err.Error(), IsError: true}
	}
	if resp.IsError {
		b.registerErr()
	}
	b.observe(CallObservation{
		Tool:     gateName,
		Args:     req.Args,
		DurMs:    time.Since(start).Milliseconds(),
		OutBytes: len(resp.Text),
		IsError:  resp.IsError,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// observe forwards one per-call record to the Observe hook, if wired.
func (b *Bridge) observe(ob CallObservation) {
	if b.cfg.Observe != nil {
		b.cfg.Observe(ob)
	}
}

// registerCall counts one call, reporting false once the per-execution cap is
// exhausted (the call is then rejected, not dispatched).
func (b *Bridge) registerCall(tool string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total >= bridgeMaxCalls {
		return false
	}
	b.total++
	b.counts[tool]++
	return true
}

func (b *Bridge) registerErr() {
	b.mu.Lock()
	b.errs++
	b.mu.Unlock()
}

func (b *Bridge) registerDenied() {
	b.mu.Lock()
	b.denied++
	b.mu.Unlock()
}

// Summary renders the per-execution MCP call accounting for the tool result —
// the observability the model (and the user reading the transcript) gets in
// place of individual tool_use cards. Empty when no calls were made.
func (b *Bridge) Summary() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total == 0 {
		return ""
	}
	names := make([]string, 0, len(b.counts))
	for n := range b.counts {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if c := b.counts[n]; c > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", n, c))
		} else {
			parts = append(parts, n)
		}
	}
	s := fmt.Sprintf("%d tool call(s): %s", b.total, strings.Join(parts, ", "))
	if b.errs > 0 {
		s += fmt.Sprintf(" (%d errored)", b.errs)
	}
	if b.denied > 0 {
		s += fmt.Sprintf(" (%d denied by permission gate)", b.denied)
	}
	return s
}

func writeBridgeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
