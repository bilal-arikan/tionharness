package mcp

import (
	"context"
	"testing"
	"time"
)

// TestPoolCatalogCachedNeverDials: before any connection exists CatalogCached
// returns instantly with every server skipped (no process start, no handshake
// wait — not even for a server whose binary does not exist); once Catalog has
// connected a server, CatalogCached serves its tool list from the pool.
func TestPoolCatalogCachedNeverDials(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)
	dead := ServerConfig{Name: "dead", Transport: MCPTransportStdio, Command: "tionharness-no-such-mcp-binary"}
	cfgs := []ServerConfig{fakeCfg(), dead}

	start := time.Now()
	entries, byServer, skipped := p.CatalogCached(cfgs)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("CatalogCached took %v; it must not dial", el)
	}
	if len(entries) != 0 || len(skipped) != 2 {
		t.Fatalf("cold pool: entries=%d skipped=%v, want 0 entries and both servers skipped", len(entries), skipped)
	}
	if _, ok := byServer["fake"]; !ok {
		t.Fatalf("cfgByServer must carry every config, got %v", byServer)
	}

	// Connect the fake server the normal way.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, _, errs := p.Catalog(ctx, []ServerConfig{fakeCfg()}); len(errs) != 0 {
		t.Fatalf("Catalog(fake) errors: %v", errs)
	}

	entries, _, skipped = p.CatalogCached(cfgs)
	if len(skipped) != 1 || skipped[0] != "dead" {
		t.Fatalf("after connect only the dead server should be skipped, got %v", skipped)
	}
	if len(entries) != 1 || entries[0].Server != "fake" || entries[0].Tool.Name != "echo" {
		t.Fatalf("cached catalog = %+v, want the fake server's echo tool", entries)
	}
}
