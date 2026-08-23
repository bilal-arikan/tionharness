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
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// poolTTL bounds how long a pooled server's cached tool list is reused before a
// refresh, as a safety net for servers that do not emit tools/list_changed. The
// refresh runs on the EXISTING persistent connection (cheap — no re-dial).
// Overridable via TIONSWARM_MCP_POOL_TTL_SEC (0 disables the timer; listChanged
// still refreshes).
const poolTTL = 60 * time.Second

// scopedIdleTTL bounds how long a SCOPED (per-session/agent) connection may sit
// idle before the reaper closes it, so a session that walked away does not pin an
// stdio subprocess forever. Shared (workspace-wide) connections are never reaped —
// they live for the workspace, as before. Overridable via
// TIONSWARM_MCP_SCOPED_IDLE_SEC (0 disables idle eviction).
const scopedIdleTTL = 300 * time.Second

// scopeSep joins a caller ScopeKey to a server name to form a scoped pool-entry
// key. It is a NUL byte, which never appears in a sanitized server name, so a
// key containing it is unambiguously scoped (used by the reaper to tell scoped
// entries apart from shared ones).
const scopeSep = "\x00"

// scopedEntryKey builds the pool-entry key for a server. An empty scopeKey keeps
// the bare server name (the shared, workspace-lifetime slot); a non-empty one
// yields a distinct per-scope slot.
func scopedEntryKey(scopeKey, server string) string {
	if scopeKey == "" {
		return server
	}
	return scopeKey + scopeSep + server
}

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
	idleTTL  time.Duration    // scoped-connection idle eviction window (0 = disabled)
	now      func() time.Time // injectable clock for tests; nil => time.Now
	onChange func()           // optional: fired (async) when any server's tools change
	logger   *slog.Logger     // optional: lifecycle logs to the in-app Logs ring buffer
	stop     chan struct{}    // closed by Close to stop the reaper goroutine
	stopOnce sync.Once
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

	// lastUsedNano is the Unix-nanos timestamp of the last ensure() on this entry.
	// Read by the reaper without holding e.mu, so it is an atomic. Only meaningful
	// for scoped entries (shared ones are never reaped).
	lastUsedNano atomic.Int64
}

// NewPool returns an empty pool with the default (env-overridable) TTLs and
// starts a background reaper that evicts idle scoped connections. Call Close to
// terminate connections and stop the reaper.
func NewPool() *Pool {
	p := &Pool{
		entries: map[string]*poolEntry{},
		ttl:     poolTTLFromEnv(),
		idleTTL: scopedIdleFromEnv(),
		stop:    make(chan struct{}),
	}
	go p.reapLoop()
	return p
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
	// Mark activity for the idle reaper (scoped entries only; harmless for shared).
	e.lastUsedNano.Store(p.clock().UnixNano())
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
		// A scoped server (cfg.ScopeKey set) gets its own per-caller slot; a shared
		// one keeps the bare server-name slot reused across the whole workspace.
		e := p.entry(scopedEntryKey(cfg.ScopeKey, name))
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
		return CallToolResult{}, unknownServerErr(server, namespaced, cfgByServer)
	}
	// Route to the same slot Catalog used: scoped (per-caller) when cfg.ScopeKey is
	// set, shared (workspace-wide) otherwise.
	e := p.entry(scopedEntryKey(cfg.ScopeKey, server))
	for attempt := 0; attempt < 2; attempt++ {
		e.mu.Lock()
		client, err := p.ensure(ctx, e, cfg)
		e.mu.Unlock()
		if err != nil {
			return CallToolResult{}, err
		}
		res, err := client.CallTool(ctx, tool, args)
		if err == nil {
			// A successful tool call may change the server's dynamic tool surface.
			// Mark the cached list stale before returning so an immediately rebuilt
			// registry cannot race the asynchronous tools/list_changed callback and
			// observe the pre-call catalog. The next Catalog refresh still reuses this
			// connection and session; only tools/list is repeated.
			e.mu.Lock()
			e.listed = false
			e.mu.Unlock()
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

// EntryStat is a read-only snapshot of one pooled connection, for observability
// (the Tools screen's live-connection / reaper indicator).
type EntryStat struct {
	Server   string `json:"server"`   // server name (recovered from the pool key)
	Scoped   bool   `json:"scoped"`   // true = per-(session,agent) connection
	ScopeKey string `json:"scopeKey"` // "" for shared; "<sessionID>|<agentID>" for scoped
	Alive    bool   `json:"alive"`    // the underlying client is connected
	IdleSec  int    `json:"idleSec"`  // seconds since last use (drives eviction)
}

// ServerState is what the pool can honestly say about one configured server's
// LIVE connection, as opposed to its mere presence in the config.
type ServerState int

const (
	// ServerUnknown: the pool has never opened a slot for this server, so it has
	// no evidence either way. This is the normal state for a server whose tool
	// loop runs OUTSIDE the pool (the claude-cli provider owns its own MCP
	// clients) and for any server before its first use in the process.
	ServerUnknown ServerState = iota
	// ServerAlive: a pooled connection exists and its client reports connected.
	ServerAlive
	// ServerDead: a slot exists but the connection is gone (dial failed, process
	// exited, read loop dead). The server is configured but NOT usable right now.
	ServerDead
)

// ServerState reports the live connection state of the named server (the stored
// server name; it is sanitized here the same way pool keys are). Scoped and
// shared slots are both considered: any live slot makes the server alive.
//
// Callers use this to avoid asserting "the server is connected" in a prompt on
// the strength of a config row alone. Note the deliberate tri-state: absence of
// a slot is ServerUnknown, never ServerDead — the pool is lazy, and treating
// "not dialed yet" as "broken" would suppress correct information.
func (p *Pool) ServerState(server string) ServerState {
	name, _, _ := SplitNamespaced(NamespaceTool(server, "x"))
	if name == "" {
		return ServerUnknown
	}
	state := ServerUnknown
	for _, s := range p.Stats() {
		if s.Server != name {
			continue
		}
		if s.Alive {
			return ServerAlive
		}
		state = ServerDead
	}
	return state
}

// Stats returns a snapshot of every pooled entry. Cheap and lock-safe; intended
// for a status endpoint, not a hot path.
func (p *Pool) Stats() []EntryStat {
	now := p.clock()
	p.mu.Lock()
	keys := make([]string, 0, len(p.entries))
	ents := make([]*poolEntry, 0, len(p.entries))
	for k, e := range p.entries {
		keys = append(keys, k)
		ents = append(ents, e)
	}
	p.mu.Unlock()

	out := make([]EntryStat, 0, len(ents))
	for i, e := range ents {
		k := keys[i]
		scoped := strings.Contains(k, scopeSep)
		server, scopeKey := k, ""
		if scoped {
			parts := strings.SplitN(k, scopeSep, 2)
			scopeKey, server = parts[0], parts[1]
		}
		e.mu.Lock()
		alive := e.client != nil && e.client.Alive()
		e.mu.Unlock()
		idle := 0
		if lu := e.lastUsedNano.Load(); lu > 0 {
			idle = int(now.Sub(time.Unix(0, lu)).Seconds())
		}
		out = append(out, EntryStat{Server: server, Scoped: scoped, ScopeKey: scopeKey, Alive: alive, IdleSec: idle})
	}
	return out
}

// IdleTTL is the scoped-connection idle-eviction window (0 = eviction disabled).
func (p *Pool) IdleTTL() time.Duration { return p.idleTTL }

// Close terminates every pooled connection and stops the reaper. Safe to call
// multiple times.
func (p *Pool) Close() {
	p.stopOnce.Do(func() { close(p.stop) })
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
// ScopeKey is caller identity (it selects the pool slot, not how we dial), so it
// is cleared before hashing — otherwise two scopes of the same server would look
// like a config change to each other.
func configFingerprint(cfg ServerConfig) string {
	cfg.ScopeKey = ""
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

func scopedIdleFromEnv() time.Duration {
	if v := os.Getenv("TIONSWARM_MCP_SCOPED_IDLE_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return scopedIdleTTL
}

// reapLoop periodically evicts idle scoped connections until Close stops it. The
// tick is half the idle window (min 30s) so an entry is closed within ~1.5× its
// idle TTL of going quiet. Disabled when idleTTL <= 0.
func (p *Pool) reapLoop() {
	if p.idleTTL <= 0 {
		return
	}
	interval := p.idleTTL / 2
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.reapScoped()
		}
	}
}

// reapScoped closes and drops every scoped entry idle beyond idleTTL. Shared
// entries (bare-name keys) are never touched — they are workspace-lifetime.
func (p *Pool) reapScoped() {
	if p.idleTTL <= 0 {
		return
	}
	cutoff := p.clock().Add(-p.idleTTL).UnixNano()
	p.mu.Lock()
	var stale []*poolEntry
	for k, e := range p.entries {
		if !strings.Contains(k, scopeSep) {
			continue // shared slot — never reaped
		}
		if e.lastUsedNano.Load() < cutoff {
			stale = append(stale, e)
			delete(p.entries, k)
		}
	}
	p.mu.Unlock()
	// Close outside the pool lock; a slow Close must not stall other callers.
	for _, e := range stale {
		e.mu.Lock()
		if e.client != nil {
			_ = e.client.Close()
			e.client = nil
			e.listed = false
		}
		e.mu.Unlock()
		p.log(slog.LevelInfo, "mcp pool: evicted idle scoped connection", "key", e.name)
	}
}
