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

// Stats reports one entry per live connection, classified shared vs scoped with
// the server name and scope key recovered from the pool key.
func TestPoolStatsClassifies(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	if _, _, errs := p.Catalog(ctx, []ServerConfig{fakeCfg()}); len(errs) != 0 {
		t.Fatalf("shared build errs: %v", errs)
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("scoped build errs: %v", errs)
	}

	stats := p.Stats()
	if len(stats) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(stats), stats)
	}
	var shared, scoped *EntryStat
	for i := range stats {
		if stats[i].Scoped {
			scoped = &stats[i]
		} else {
			shared = &stats[i]
		}
	}
	if shared == nil || scoped == nil {
		t.Fatalf("want one shared + one scoped, got %+v", stats)
	}
	if shared.Server != "fake" || shared.ScopeKey != "" || !shared.Alive {
		t.Errorf("shared entry wrong: %+v", *shared)
	}
	if scoped.Server != "fake" || scoped.ScopeKey != "s1|a1" || !scoped.Alive {
		t.Errorf("scoped entry wrong: %+v", *scoped)
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

// CloseSession closes every connection scoped to the named session — across all
// of that session's agents — and leaves other sessions and the shared slot alone.
// Session deletion depends on this: a scoped stdio server's cwd is the session's
// scratchpad, and Windows will not remove a directory a live process sits in.
func TestPoolCloseSessionClosesOnlyThatSession(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	for _, scope := range []string{"s1|a1", "s1|a2", "s2|a1"} {
		if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg(scope)}); len(errs) != 0 {
			t.Fatalf("%s build errs: %v", scope, errs)
		}
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{fakeCfg()}); len(errs) != 0 {
		t.Fatalf("shared build errs: %v", errs)
	}
	doomedA1 := p.entry(scopedEntryKey("s1|a1", "fake")).client
	doomedA2 := p.entry(scopedEntryKey("s1|a2", "fake")).client
	survivor := p.entry(scopedEntryKey("s2|a1", "fake")).client
	shared := p.entry("fake").client

	if closed := p.CloseSession("s1"); closed != 2 {
		t.Fatalf("want 2 closed connections for s1, got %d", closed)
	}

	for name, key := range map[string]string{
		"s1|a1": scopedEntryKey("s1|a1", "fake"),
		"s1|a2": scopedEntryKey("s1|a2", "fake"),
	} {
		if _, ok := p.entries[key]; ok {
			t.Errorf("%s entry must be dropped from the pool", name)
		}
	}
	if doomedA1.Alive() || doomedA2.Alive() {
		t.Error("every connection of the deleted session must be closed")
	}
	if _, ok := p.entries[scopedEntryKey("s2|a1", "fake")]; !ok || !survivor.Alive() {
		t.Error("another session's connection must survive")
	}
	if _, ok := p.entries["fake"]; !ok || !shared.Alive() {
		t.Error("the shared connection must survive")
	}
}

// A session id must match on the full "<sessionID>|" boundary, so closing SES1
// never takes SES11 down with it.
func TestPoolCloseSessionDoesNotMatchIDPrefix(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	for _, scope := range []string{"SES1|a1", "SES11|a1"} {
		if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg(scope)}); len(errs) != 0 {
			t.Fatalf("%s build errs: %v", scope, errs)
		}
	}
	neighbour := p.entry(scopedEntryKey("SES11|a1", "fake")).client

	if closed := p.CloseSession("SES1"); closed != 1 {
		t.Fatalf("want exactly 1 closed connection, got %d", closed)
	}
	if _, ok := p.entries[scopedEntryKey("SES11|a1", "fake")]; !ok || !neighbour.Alive() {
		t.Fatal("SES11 must not be closed by deleting SES1")
	}
}

// An empty session id closes nothing: it would otherwise be a prefix of every
// scope key and take the whole pool down.
func TestPoolCloseSessionEmptyIsNoOp(t *testing.T) {
	ctx := context.Background()
	p := newTestPool(scopedIdleTTL, nil)
	defer p.Close()

	if _, _, errs := p.Catalog(ctx, []ServerConfig{scopedCfg("s1|a1")}); len(errs) != 0 {
		t.Fatalf("scoped build errs: %v", errs)
	}
	if closed := p.CloseSession(""); closed != 0 {
		t.Fatalf("empty session id must close nothing, closed %d", closed)
	}
	if !p.entry(scopedEntryKey("s1|a1", "fake")).client.Alive() {
		t.Fatal("connection must stay alive")
	}
}
