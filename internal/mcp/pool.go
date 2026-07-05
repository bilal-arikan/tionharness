package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

// poolTTL bounds how long a pooled server's cached tool list is reused before a
// refresh, as a safety net for servers that do not emit tools/list_changed. The
// refresh runs on the EXISTING persistent connection (cheap — no re-dial).
// Overridable via TIONSWARM_MCP_POOL_TTL_SEC (0 disables the timer; listChanged
// still refreshes).
const poolTTL = 60 * time.Second

// Pool maintains one persistent stdio client per MCP server (keyed by sanitized
// server name). Persistent connections — as opposed to the old dial-per-
// operation model — give three properties TionSwarm needs:
//
//   - The repeated per-turn catalog builds reuse a live session instead of
//     opening (and leaking) a fresh one every time — no session churn on
//     HTTP-bridged servers such as the local MCP gateway.
//   - Session-scoped server state survives across calls: the gateway's
//     activate_tools effect persists, so a later tools/list (and tool call) on
//     the same session sees the newly activated tools.
//   - tools/list_changed notifications invalidate the cached tool list, so a
//     dynamic server's catalog refreshes on the next build automatically.
//
// A server whose config (command/args/url/env) changes is transparently
// re-dialed; a dead connection is re-dialed on next use. The zero value is not
// usable — call NewPool.
type Pool struct {
	mu       sync.Mutex
	entries  map[string]*poolEntry
	ttl      time.Duration
	now      func() time.Time // injectable clock for tests; nil => time.Now
	onChange func()           // optional: fired (async) when any server's tools change
	logger   *slog.Logger     // optional: lifecycle logs to the in-app Logs ring buffer
}

type poolEntry struct {
	name string

	mu       sync.Mutex
	cfg      ServerConfig
	fp       string
	client   Client
	tools    []Tool
	listed   bool
	listedAt time.Time
}

// NewPool returns an empty pool with the default (env-overridable) TTL.
func NewPool() *Pool {
	return &Pool{entries: map[string]*poolEntry{}, ttl: poolTTLFromEnv()}
}

// SetOnToolsChanged registers a callback fired (in its own goroutine) whenever
// any pooled server announces a tool-list change. Lets a higher layer react
// (e.g. nudge a refresh). Safe to leave unset.
func (p *Pool) SetOnToolsChanged(fn func()) {
	p.mu.Lock()
	p.onChange = fn
	p.mu.Unlock()
}

// SetLogger attaches a logger for connection-lifecycle events (dial, re-dial on
// config change / death, tools/list_changed). Optional and nil-safe — the pool
// stays silent until one is set. The MCP persistent pool is otherwise opaque, so
// these logs surface session churn / reconnects in the in-app Logs screen.
func (p *Pool) SetLogger(l *slog.Logger) {
	p.mu.Lock()
	p.logger = l
	p.mu.Unlock()
}

// log emits at the given level via the attached logger, if any. Nil-safe.
func (p *Pool) log(level slog.Level, msg string, args ...any) {
	p.mu.Lock()
	l := p.logger
	p.mu.Unlock()
	if l != nil {
		l.Log(context.Background(), level, msg, args...)
	}
}

func (p *Pool) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// entry returns the per-server slot, creating an empty one on first use.
func (p *Pool) entry(name string) *poolEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entries[name]
	if e == nil {
		e = &poolEntry{name: name}
		p.entries[name] = e
	}
	return e
}

// ensure returns a live client for cfg, dialing (or re-dialing on config change
// / death) as needed. Caller holds e.mu.
func (p *Pool) ensure(ctx context.Context, e *poolEntry, cfg ServerConfig) (Client, error) {
	want := configFingerprint(cfg)
	if e.client != nil && e.fp == want && e.client.Alive() {
		return e.client, nil
	}
	// Classify why we (re)dial so the log distinguishes a routine first connect
	// from a config-driven reconnect or a recovered dead connection.
	redial := "first_connect"
	if e.client != nil {
		if e.fp != want {
			redial = "config_changed"
		} else {
			redial = "connection_dead"
		}
		_ = e.client.Close()
		e.client = nil
		e.listed = false
	}
	client, err := cfg.dial(ctx)
	if err != nil {
		p.log(slog.LevelWarn, "mcp pool: dial failed", "server", cfg.Name, "reason", redial, "error", err.Error())
		return nil, err
	}
	name := e.name
	client.SetOnToolsChanged(func() { p.invalidate(name) })
	// Hand the client the pool's logger so its read loop can report why a
	// connection later dies (the pool only sees the symptom, not the cause).
	p.mu.Lock()
	l := p.logger
	p.mu.Unlock()
	client.SetLogger(l, cfg.Name)
	e.client = client
	e.cfg = cfg
	e.fp = want
	e.listed = false
	p.log(slog.LevelInfo, "mcp pool: connected", "server", cfg.Name, "reason", redial)
	return client, nil
}

// invalidate marks a server's cached tool list stale (on tools/list_changed) and
// signals the optional pool-level callback.
func (p *Pool) invalidate(name string) {
	e := p.entry(name)
	e.mu.Lock()
	e.listed = false
	e.mu.Unlock()
	p.log(slog.LevelDebug, "mcp pool: tool list invalidated (list_changed)", "server", name)
	p.mu.Lock()
	fn := p.onChange
	p.mu.Unlock()
	if fn != nil {
		go fn()
	}
}

// tools returns cfg's tool list from the live session, refreshing when stale
// (never listed, listChanged-invalidated, or past the TTL). Caller holds e.mu.
func (p *Pool) tools(ctx context.Context, e *poolEntry, cfg ServerConfig) ([]Tool, error) {
	client, err := p.ensure(ctx, e, cfg)
	if err != nil {
		return nil, err
	}
	fresh := e.listed
	if fresh && p.ttl > 0 && p.clock().Sub(e.listedAt) >= p.ttl {
		fresh = false
	}
	if fresh {
		return e.tools, nil
	}
	list, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	e.tools = list
	e.listed = true
	e.listedAt = p.clock()
	return list, nil
}

// Catalog returns the namespaced tool catalog and the dispatch map for the given
// enabled servers, reusing live connections. A server that fails to connect is
// skipped and its error recorded in errs (keyed by server name).
func (p *Pool) Catalog(ctx context.Context, cfgs []ServerConfig) (entries []CatalogEntry, cfgByServer map[string]ServerConfig, errs map[string]string) {
	errs = map[string]string{}
	cfgByServer = map[string]ServerConfig{}
	for _, cfg := range cfgs {
		name, _, _ := SplitNamespaced(NamespaceTool(cfg.Name, "x"))
		cfgByServer[name] = cfg
		e := p.entry(name)
		e.mu.Lock()
		list, err := p.tools(ctx, e, cfg)
		e.mu.Unlock()
		if err != nil {
			errs[cfg.Name] = err.Error()
			continue
		}
		for _, t := range list {
			entries = append(entries, CatalogEntry{
				Server:         cfg.Name,
				NamespacedName: NamespaceTool(cfg.Name, t.Name),
				Tool:           t,
			})
		}
	}
	return entries, cfgByServer, errs
}

// Call dispatches a namespaced tool call over the owning server's persistent
// connection. cfgByServer maps the sanitized server name to its config. A call
// that fails on a dead connection is retried once after a re-dial.
func (p *Pool) Call(ctx context.Context, cfgByServer map[string]ServerConfig, namespaced string, args json.RawMessage) (CallToolResult, error) {
	server, tool, ok := SplitNamespaced(namespaced)
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: %q is not a namespaced tool name", namespaced)
	}
	cfg, ok := cfgByServer[server]
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: no server %q for tool %q", server, namespaced)
	}
	e := p.entry(server)
	for attempt := 0; attempt < 2; attempt++ {
		e.mu.Lock()
		client, err := p.ensure(ctx, e, cfg)
		e.mu.Unlock()
		if err != nil {
			return CallToolResult{}, err
		}
		res, err := client.CallTool(ctx, tool, args)
		if err == nil {
			return res, nil
		}
		// A dead connection (read loop gone) is worth one transparent re-dial;
		// any other error (or ctx cancellation) is returned as-is.
		if attempt == 0 && !client.Alive() && ctx.Err() == nil {
			p.log(slog.LevelWarn, "mcp pool: call hit dead connection, re-dialing", "server", server, "tool", tool, "error", err.Error())
			continue
		}
		return res, err
	}
	return CallToolResult{}, ctx.Err()
}

// Close terminates every pooled connection. Safe to call multiple times.
func (p *Pool) Close() {
	p.mu.Lock()
	es := make([]*poolEntry, 0, len(p.entries))
	for _, e := range p.entries {
		es = append(es, e)
	}
	p.entries = map[string]*poolEntry{}
	p.mu.Unlock()
	for _, e := range es {
		e.mu.Lock()
		if e.client != nil {
			_ = e.client.Close()
			e.client = nil
		}
		e.mu.Unlock()
	}
}

// configFingerprint hashes a server's launch/connection spec; a change re-dials.
func configFingerprint(cfg ServerConfig) string {
	h := sha256.New()
	enc := json.NewEncoder(h) // encoding/json sorts map keys → deterministic Env
	_ = enc.Encode(cfg)
	return hex.EncodeToString(h.Sum(nil))
}

func poolTTLFromEnv() time.Duration {
	if v := os.Getenv("TIONSWARM_MCP_POOL_TTL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return poolTTL
}
