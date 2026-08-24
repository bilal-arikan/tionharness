package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// ServersFunc returns the enabled MCP servers of a WORKSPACE the gateway may expose
// (workspaceID "" = default workspace). Read live so a config change is picked up without
// a gateway restart, mirroring the TS gateway's readConfig-per-call behaviour.
type ServersFunc func(ctx context.Context, workspaceID string) ([]mcp.ServerConfig, error)

// PoolFunc returns a WORKSPACE's MCP connection pool to dispatch through (workspaceID ""
// = default). Resolved live so the gateway SHARES that workspace's persistent pool (#11) —
// internal agent turns and external gateway clients reuse one backend connection per
// server. May return nil (no workspace) → the backend reports a clean error.
type PoolFunc func(workspaceID string) *mcp.Pool

// AuditFunc records one proxied backend tool call (gateway-audit parity with the TS
// gateway's gateway-audit.jsonl). Fire-and-forget; nil disables auditing.
type AuditFunc func(AuditEntry)

// mcpBackend implements Backend over TionHarness's mcp.Pool: it starts each session with
// meta-tools only and grows the surface when the client activates a backend server.
type mcpBackend struct {
	poolFn  PoolFunc
	servers ServersFunc
	audit   AuditFunc
	srv     *Server // set via SetServer, for tools/list_changed pushes
	logger  *slog.Logger

	mu       sync.Mutex
	sessions map[string]*gwSession
}

type gwSession struct {
	// workspaceID is the workspace this client session is bound to ("" = default).
	workspaceID string
	// activated maps a sanitized server name to its advertised namespaced tools.
	activated map[string][]ToolSpec
	// cfgByServer is the accumulated pool dispatch map (sanitized server -> config).
	cfgByServer map[string]mcp.ServerConfig
}

// NewBackend builds the gateway backend. poolFn/servers resolve a workspace's shared pool
// + enabled servers live. Wire the *Server's pusher with SetServer after constructing it.
func NewBackend(poolFn PoolFunc, servers ServersFunc, logger *slog.Logger) *mcpBackend {
	if logger == nil {
		logger = slog.Default()
	}
	return &mcpBackend{poolFn: poolFn, servers: servers, logger: logger, sessions: map[string]*gwSession{}}
}

// SetServer wires the streaming server so activate can push tools/list_changed.
func (b *mcpBackend) SetServer(s *Server) { b.srv = s }

// SetAudit wires an audit sink for proxied tool calls (gateway-audit parity). nil disables.
func (b *mcpBackend) SetAudit(fn AuditFunc) { b.audit = fn }

// OpenSession binds a new session to a workspace (from the X-Workspace-Id header).
func (b *mcpBackend) OpenSession(sid, workspaceID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sessions[sid]
	if s == nil {
		s = &gwSession{activated: map[string][]ToolSpec{}, cfgByServer: map[string]mcp.ServerConfig{}}
		b.sessions[sid] = s
	}
	s.workspaceID = workspaceID
}

func (b *mcpBackend) session(sid string) *gwSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sessions[sid]
	if s == nil {
		s = &gwSession{activated: map[string][]ToolSpec{}, cfgByServer: map[string]mcp.ServerConfig{}}
		b.sessions[sid] = s
	}
	return s
}

// workspaceOf returns the workspace id a session is bound to.
func (b *mcpBackend) workspaceOf(sid string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s := b.sessions[sid]; s != nil {
		return s.workspaceID
	}
	return ""
}

// pool resolves a session's workspace pool, or nil when none is ready.
func (b *mcpBackend) pool(sid string) *mcp.Pool {
	if b.poolFn == nil {
		return nil
	}
	return b.poolFn(b.workspaceOf(sid))
}

// serversFor resolves a session's workspace's enabled servers.
func (b *mcpBackend) serversFor(ctx context.Context, sid string) ([]mcp.ServerConfig, error) {
	if b.servers == nil {
		return nil, nil
	}
	return b.servers(ctx, b.workspaceOf(sid))
}

// CloseSession drops a session's activation state. The pool's backend connections are
// shared and persistent (workspace-lifetime), so they are NOT closed here.
func (b *mcpBackend) CloseSession(sid string) {
	b.mu.Lock()
	delete(b.sessions, sid)
	b.mu.Unlock()
}

var gatewayMetaTools = []ToolSpec{
	{Name: "list_servers", Description: "List the MCP servers this gateway can expose, with whether each is active in this session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
	{Name: "activate_tools", Description: "Connect to backend MCP servers and register their tools in THIS session (context grows only for what you activate). Pass server names in `servers`.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"servers":{"type":"array","items":{"type":"string"},"description":"Server names to activate"}},"required":["servers"]}`)},
	{Name: "deactivate_tools", Description: "Remove a backend server's tools from THIS session to free context. Pass server names in `servers`.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"servers":{"type":"array","items":{"type":"string"},"description":"Server names to deactivate"}},"required":["servers"]}`)},
	{Name: "active_tools", Description: "List the backend servers (and tool counts) currently active in this session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
}

// Tools returns the meta-tools plus every activated server's namespaced tools.
func (b *mcpBackend) Tools(sid string) []ToolSpec {
	out := append([]ToolSpec(nil), gatewayMetaTools...)
	s := b.session(sid)
	b.mu.Lock()
	defer b.mu.Unlock()
	names := make([]string, 0, len(s.activated))
	for name := range s.activated {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, s.activated[name]...)
	}
	return out
}

// Call dispatches a meta-tool or proxies a namespaced backend tool call.
func (b *mcpBackend) Call(ctx context.Context, sid, name string, args json.RawMessage) (CallResult, error) {
	switch name {
	case "list_servers":
		return b.callListServers(ctx, sid)
	case "activate_tools":
		return b.callActivate(ctx, sid, args, true)
	case "deactivate_tools":
		return b.callActivate(ctx, sid, args, false)
	case "active_tools":
		return b.callActiveTools(sid)
	}
	// Otherwise it is a proxied backend tool (namespaced server__tool).
	pool := b.pool(sid)
	if pool == nil {
		return CallResult{Text: "gateway has no active workspace pool", IsError: true}, nil
	}
	s := b.session(sid)
	b.mu.Lock()
	cfgByServer := s.cfgByServer
	b.mu.Unlock()
	res, err := pool.Call(ctx, cfgByServer, name, args)
	b.recordAudit(name, err, res.IsError) // gateway-audit parity (proxied calls only)
	if err != nil {
		return CallResult{Text: "call failed: " + err.Error(), IsError: true}, nil
	}
	return CallResult{Text: res.Text, IsError: res.IsError}, nil
}

func (b *mcpBackend) callListServers(ctx context.Context, sid string) (CallResult, error) {
	cfgs, err := b.serversFor(ctx, sid)
	if err != nil {
		return CallResult{Text: "list_servers failed: " + err.Error(), IsError: true}, nil
	}
	s := b.session(sid)
	b.mu.Lock()
	active := map[string]bool{}
	for name := range s.activated {
		active[name] = true
	}
	b.mu.Unlock()
	if len(cfgs) == 0 {
		return CallResult{Text: "No MCP servers are enabled for this gateway."}, nil
	}
	var lines []string
	for _, c := range cfgs {
		san := sanitizeServer(c.Name)
		status := "available"
		if active[san] {
			status = "active"
		}
		transport := c.Transport
		if transport == "" {
			if c.URL != "" {
				transport = "http"
			} else {
				transport = "stdio"
			}
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, %s)", c.Name, status, transport))
	}
	return CallResult{Text: fmt.Sprintf("MCP servers (%d):\n%s", len(cfgs), strings.Join(lines, "\n"))}, nil
}

func (b *mcpBackend) callActivate(ctx context.Context, sid string, args json.RawMessage, activate bool) (CallResult, error) {
	var in struct {
		Servers []string `json:"servers"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return CallResult{Text: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if len(in.Servers) == 0 {
		return CallResult{Text: "no server names given", IsError: true}, nil
	}
	pool := b.pool(sid)
	if pool == nil {
		return CallResult{Text: "gateway has no active workspace pool", IsError: true}, nil
	}
	cfgs, err := b.serversFor(ctx, sid)
	if err != nil {
		return CallResult{Text: "server list failed: " + err.Error(), IsError: true}, nil
	}
	byName := map[string]mcp.ServerConfig{}
	for _, c := range cfgs {
		byName[c.Name] = c
		byName[sanitizeServer(c.Name)] = c
	}

	s := b.session(sid)
	if !activate {
		removed := b.deactivate(s, in.Servers)
		if len(removed) > 0 && b.srv != nil {
			b.srv.PushToolsChanged(sid)
		}
		if len(removed) == 0 {
			return CallResult{Text: "no servers deactivated (none were active)"}, nil
		}
		return CallResult{Text: "deactivated: " + strings.Join(removed, ", ")}, nil
	}

	var okd, unknown, failed []string
	changed := false
	for _, req := range in.Servers {
		cfg, ok := byName[req]
		if !ok {
			unknown = append(unknown, req)
			continue
		}
		entries, cfgByServer, errs := pool.Catalog(ctx, []mcp.ServerConfig{cfg})
		if len(errs) > 0 {
			for _, e := range errs {
				failed = append(failed, req+" ("+e+")")
			}
			continue
		}
		san := sanitizeServer(cfg.Name)
		specs := make([]ToolSpec, 0, len(entries))
		for _, e := range entries {
			specs = append(specs, ToolSpec{Name: e.NamespacedName, Description: "[" + cfg.Name + "] " + e.Tool.Description, InputSchema: e.Tool.InputSchema})
		}
		b.mu.Lock()
		s.activated[san] = specs
		for k, v := range cfgByServer {
			s.cfgByServer[k] = v
		}
		b.mu.Unlock()
		okd = append(okd, fmt.Sprintf("%s (%d tools)", cfg.Name, len(specs)))
		changed = true
	}
	if changed && b.srv != nil {
		b.srv.PushToolsChanged(sid)
	}

	var parts []string
	if len(okd) > 0 {
		parts = append(parts, "activated: "+strings.Join(okd, ", "))
	}
	if len(unknown) > 0 {
		parts = append(parts, "unknown: "+strings.Join(unknown, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing activated")
	}
	return CallResult{Text: strings.Join(parts, "\n"), IsError: len(okd) == 0}, nil
}

func (b *mcpBackend) deactivate(s *gwSession, servers []string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var removed []string
	for _, req := range servers {
		san := sanitizeServer(req)
		if _, ok := s.activated[san]; ok {
			delete(s.activated, san)
			delete(s.cfgByServer, san)
			removed = append(removed, req)
		}
	}
	return removed
}

func (b *mcpBackend) callActiveTools(sid string) (CallResult, error) {
	s := b.session(sid)
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(s.activated) == 0 {
		return CallResult{Text: "No servers active. Use activate_tools to connect one."}, nil
	}
	names := make([]string, 0, len(s.activated))
	for name := range s.activated {
		names = append(names, name)
	}
	sort.Strings(names)
	var lines []string
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- %s (%d tools)", name, len(s.activated[name])))
	}
	return CallResult{Text: "Active servers:\n" + strings.Join(lines, "\n")}, nil
}

// sanitizeServer derives the pool's sanitized server key from a display name, matching
// how the pool keys cfgByServer (SplitNamespaced(NamespaceTool(name,"x"))).
func sanitizeServer(name string) string {
	san, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(name, "x"))
	return san
}
