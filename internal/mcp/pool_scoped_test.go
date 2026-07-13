package mcp

import (
	"context"
	"testing"
	"time"
)

// newTestPool builds a pool WITHOUT the background reaper goroutine and with an
// injectable clock, so scoped-eviction logic can be driven deterministically.
func newTestPool(idle time.Duration, now func() time.Time) *Pool {
	return &Pool{
		entries: map[string]*poolEntry{},
		ttl:     poolTTL,
		idleTTL: idle,
		now:     now,
		stop:    make(chan struct{}),
	}
}

func scopedCfg(scopeKey string) ServerConfig {
	c := fakeCfg()
	c.ScopeKey = scopeKey
	return c
}

// A scoped server gets a distinct live connection per ScopeKey, while a shared
// server (empty ScopeKey) keeps the bare-name slot.
func TestPoolScopedIsolatesConnections(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("s1 build errs: %v", errs)
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s2|a1")}); len(errs) != 0 {
		t.Fatalf("s2 build errs: %v", errs)
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{fakeCfg()}); len(errs) != 0 {
		t.Fatalf("shared build errs: %v", errs)
	}

	e1 := p.entry(scopedEntryKey("s1|a1", "fake"))
	e2 := p.entry(scopedEntryKey("s2|a1", "fake"))
	eShared := p.entry("fake")

	if e1.client == nil || e2.client == nil || eShared.client == nil {
		t.Fatal("each scope + shared should have a live client")
	}
	if e1.client == e2.client {
		t.Fatal("distinct ScopeKeys must not share a connection")
	}
	if e1.client == eShared.client || e2.client == eShared.client {
		t.Fatal("scoped connections must be separate from the shared one")
	}
}

// The same ScopeKey reuses its connection across builds (no per-turn churn).
func TestPoolScopedReusesWithinScope(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("first build errs: %v", errs)
	}
	c1 := p.entry(scopedEntryKey("s1|a1", "fake")).client
	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("second build errs: %v", errs)
	}
	if p.entry(scopedEntryKey("s1|a1", "fake")).client != c1 {
		t.Fatal("same scope must reuse the live connection")
	}
}

// reapScoped closes and drops idle scoped entries but never the shared one.
func TestPoolReapEvictsIdleScopedOnly(t *testing.T) {
	ctx := context.Background()
	nowNano := time.Unix(1_000_000, 0)
	clk := &nowNano
	p := newTestPool(50*time.Millisecond, func() time.Time { return *clk })
	defer p.Close()

	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("scoped build errs: %v", errs)
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{fakeCfg()}); len(errs) != 0 {
		t.Fatalf("shared build errs: %v", errs)
	}
	scoped := p.entry(scopedEntryKey("s1|a1", "fake"))
	shared := p.entry("fake")
	scopedClient := scoped.client
	if scopedClient == nil || shared.client == nil {
		t.Fatal("expected live clients before reap")
	}

	// Not yet idle → nothing evicted.
	p.reapScoped()
	if _, ok := p.entries[scopedEntryKey("s1|a1", "fake")]; !ok {
		t.Fatal("scoped entry evicted too early")
	}

	// Advance the clock past the idle window → scoped entry reaped, shared kept.
	*clk = clk.Add(time.Second)
	p.reapScoped()

	if _, ok := p.entries[scopedEntryKey("s1|a1", "fake")]; ok {
		t.Fatal("idle scoped entry should have been evicted")
	}
	if scopedClient.Alive() {
		t.Fatal("evicted scoped connection should be closed")
	}
	if _, ok := p.entries["fake"]; !ok {
		t.Fatal("shared entry must never be reaped")
	}
	if !shared.client.Alive() {
		t.Fatal("shared connection must stay alive")
	}
}
