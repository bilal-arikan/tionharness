package agent

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/mcp"
)

// stdioServer builds an enabled stdio MCP server row whose command does not
// exist, so dialing it fails fast (no subprocess, no network) and BuildCatalog
// records the failure in errs. errs being non-nil is our observable proxy for
// "a fresh dial happened" (a cache hit returns nil errs).
func stdioServer(name, command string) db.MCPServer {
	return db.MCPServer{
		Name: name, Transport: "stdio", Command: command,
		Args: "[]", EnvConfig: "{}", Enabled: true,
	}
}

func TestMCPCatalogCachesAndInvalidates(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := &mcpCatalog{ttl: time.Minute, now: func() time.Time { return now }}
	ctx := context.Background()
	servers := []db.MCPServer{stdioServer("x", "swarmgo-no-such-cmd-1")}

	// First build: cache miss → dials → error recorded.
	if _, _, errs := c.build(ctx, servers); len(errs) == 0 {
		t.Fatal("first build: expected a dial error (cache miss), got none")
	}
	// Same config, within TTL: cache hit → no dial → nil errs.
	if _, _, errs := c.build(ctx, servers); errs != nil {
		t.Fatalf("second build: expected cache hit (nil errs), got %v", errs)
	}
	// Config change → fingerprint changes → cache miss again.
	servers[0].Command = "swarmgo-no-such-cmd-2"
	if _, _, errs := c.build(ctx, servers); len(errs) == 0 {
		t.Fatal("after config change: expected a dial (cache miss), got none")
	}
	// Advance the clock past the TTL → cache miss even with identical config.
	now = now.Add(2 * time.Minute)
	if _, _, errs := c.build(ctx, servers); len(errs) == 0 {
		t.Fatal("after TTL expiry: expected a dial (cache miss), got none")
	}
}

func TestMCPCatalogDisabledTTLAlwaysDials(t *testing.T) {
	c := &mcpCatalog{ttl: 0} // caching off
	ctx := context.Background()
	servers := []db.MCPServer{stdioServer("x", "swarmgo-no-such-cmd")}
	for i := 0; i < 2; i++ {
		if _, _, errs := c.build(ctx, servers); len(errs) == 0 {
			t.Fatalf("build %d: ttl=0 must always dial (non-nil errs)", i)
		}
	}
}

func TestMCPCatalogNilReceiverSafe(t *testing.T) {
	var c *mcpCatalog // bare Runtime in tests leaves mcpCat nil
	_, cfgByServer, errs := c.build(context.Background(), []db.MCPServer{stdioServer("x", "swarmgo-no-such-cmd")})
	if len(errs) == 0 {
		t.Fatal("nil cache must still dial")
	}
	if _, ok := cfgByServer["x"]; !ok {
		t.Fatalf("nil cache must still return the dispatch map, got %v", cfgByServer)
	}
}

func TestCatalogFingerprintOrderIndependent(t *testing.T) {
	a := []mcp.ServerConfig{{Name: "a", Command: "c1"}, {Name: "b", Command: "c2"}}
	b := []mcp.ServerConfig{{Name: "b", Command: "c2"}, {Name: "a", Command: "c1"}}
	if catalogFingerprint(a) != catalogFingerprint(b) {
		t.Fatal("fingerprint should be independent of server order")
	}
	// A change to any field must change the fingerprint.
	c := []mcp.ServerConfig{{Name: "a", Command: "c1"}, {Name: "b", Command: "CHANGED"}}
	if catalogFingerprint(a) == catalogFingerprint(c) {
		t.Fatal("fingerprint should change when a server config changes")
	}
}

func TestCatalogTTLFromEnv(t *testing.T) {
	t.Setenv("SWARMGO_MCP_CATALOG_TTL_SEC", "5")
	if got := catalogTTLFromEnv(); got != 5*time.Second {
		t.Fatalf("ttl = %v, want 5s", got)
	}
	t.Setenv("SWARMGO_MCP_CATALOG_TTL_SEC", "0")
	if got := catalogTTLFromEnv(); got != 0 {
		t.Fatalf("ttl = %v, want 0 (disabled)", got)
	}
	t.Setenv("SWARMGO_MCP_CATALOG_TTL_SEC", "bogus")
	if got := catalogTTLFromEnv(); got != mcpCatalogTTL {
		t.Fatalf("ttl = %v, want default %v on invalid input", got, mcpCatalogTTL)
	}
}
