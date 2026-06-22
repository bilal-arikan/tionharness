package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/mcp"
)

// mcpCatalogTTL bounds how long a built MCP catalog is reused before its servers
// are re-dialed. Short enough that a tool-list change on the server side surfaces
// quickly; long enough that the per-turn buildRegistry burst (turn entry + native
// loop + UI/tool previews) dials each server once instead of N times. Overridable
// via SWARMGO_MCP_CATALOG_TTL_SEC (0 disables caching → original dial-every-build
// behavior).
const mcpCatalogTTL = 60 * time.Second

// mcpCatalog memoizes the result of dialing every enabled MCP server and listing
// its tools. Without it, buildRegistry re-dials (initialize + tools/list + close)
// every enabled server on each call — several times per chat turn — which, for an
// HTTP-bridged server such as the local MCP gateway, opens a brand-new session on
// every dial and leaves idle sessions piling up gateway-side.
//
// The cache key is a fingerprint of the enabled server configs, so toggling,
// adding, removing or editing a server invalidates the cache immediately (no
// explicit invalidation hook needed). The TTL bounds staleness from changes that
// the config alone cannot observe (a server advertising different tools).
//
// Only the expensive dial result (entries) is cached; the cheap dispatch map
// (cfgByServer) is recomputed from the live server list on every call so it can
// never drift from the currently enabled set.
type mcpCatalog struct {
	ttl time.Duration
	now func() time.Time // injectable clock for tests; nil => time.Now

	mu          sync.Mutex
	fingerprint string
	entries     []mcp.CatalogEntry
	built       bool
	expiry      time.Time
}

// newMCPCatalog builds a catalog cache with the default TTL (honoring the
// SWARMGO_MCP_CATALOG_TTL_SEC override).
func newMCPCatalog() *mcpCatalog {
	return &mcpCatalog{ttl: catalogTTLFromEnv()}
}

// clock returns the current time, using the injected clock when set (tests).
func (c *mcpCatalog) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}

// build returns the namespaced tool catalog and the per-server dispatch map for
// the given enabled servers, dialing them only when the cache is empty, expired,
// or the enabled-server set has changed. A nil receiver (or a zero TTL) disables
// caching and always dials. errs is populated only on a fresh build.
func (c *mcpCatalog) build(ctx context.Context, servers []db.MCPServer) (entries []mcp.CatalogEntry, cfgByServer map[string]mcp.ServerConfig, errs map[string]string) {
	cfgs := make([]mcp.ServerConfig, 0, len(servers))
	cfgByServer = map[string]mcp.ServerConfig{}
	for _, m := range servers {
		cfg := toServerConfig(m)
		cfgs = append(cfgs, cfg)
		// Key by the sanitized name used in tool namespacing.
		srv, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(cfg.Name, "x"))
		cfgByServer[srv] = cfg
	}

	// No cache (bare Runtime in tests) or caching disabled: dial every time.
	if c == nil || c.ttl <= 0 {
		entries, errs = mcp.BuildCatalog(ctx, cfgs)
		return entries, cfgByServer, errs
	}

	fp := catalogFingerprint(cfgs)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.built && c.fingerprint == fp && c.clock().Before(c.expiry) {
		return c.entries, cfgByServer, nil
	}
	entries, errs = mcp.BuildCatalog(ctx, cfgs)
	c.fingerprint = fp
	c.entries = entries
	c.built = true
	c.expiry = c.clock().Add(c.ttl)
	return entries, cfgByServer, errs
}

// catalogFingerprint produces a stable, order-independent hash of the enabled
// server configs. Any change to a server's launch/connection spec (or the set of
// enabled servers) changes the fingerprint and thus invalidates the cache.
func catalogFingerprint(cfgs []mcp.ServerConfig) string {
	sorted := append([]mcp.ServerConfig(nil), cfgs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	enc := json.NewEncoder(h) // encoding/json sorts map keys → deterministic Env
	for _, cfg := range sorted {
		_ = enc.Encode(cfg)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// catalogTTLFromEnv reads SWARMGO_MCP_CATALOG_TTL_SEC; a valid non-negative value
// overrides the default (0 disables caching). Anything invalid keeps the default.
func catalogTTLFromEnv() time.Duration {
	if v := os.Getenv("SWARMGO_MCP_CATALOG_TTL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return mcpCatalogTTL
}
