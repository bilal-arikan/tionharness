package mcp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// disconnectRecorder collects the pool's disconnect events. The pool fires the
// hook from the dying client's goroutine, so access is mutex-guarded.
type disconnectRecorder struct {
	mu     sync.Mutex
	events []DisconnectEvent
}

func (r *disconnectRecorder) record(ev DisconnectEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *disconnectRecorder) list() []DisconnectEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]DisconnectEvent(nil), r.events...)
}

// dieableServer is a config whose dial hands back a fakeClient the test keeps a
// handle on, so it can kill the connection under the pool's feet.
func dieableServer(name string, got *[]*fakeClient, mu *sync.Mutex) ServerConfig {
	return ServerConfig{
		Name:    name,
		Command: "runs",
		stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			c := &fakeClient{tools: []Tool{{Name: "t"}}}
			mu.Lock()
			*got = append(*got, c)
			mu.Unlock()
			return c, nil
		},
	}
}

// poolWithDieableServer wires a recorder and returns the pool plus an accessor
// for the clients it dialed.
func poolWithDieableServer(t *testing.T, cfgName string) (*Pool, *disconnectRecorder, func() []*fakeClient, ServerConfig) {
	t.Helper()
	rec := &disconnectRecorder{}
	var mu sync.Mutex
	var clients []*fakeClient
	p := newTestPool(scopedIdleTTL, nil)
	t.Cleanup(p.Close)
	p.SetOnDisconnect(rec.record)
	cfg := dieableServer(cfgName, &clients, &mu)
	return p, rec, func() []*fakeClient {
		mu.Lock()
		defer mu.Unlock()
		return append([]*fakeClient(nil), clients...)
	}, cfg
}

// waitForEvents polls until n events arrive or the deadline passes. The hook is
// fired from another goroutine, so a bare read would race.
func waitForEvents(t *testing.T, rec *disconnectRecorder, n int) []DisconnectEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if evs := rec.list(); len(evs) >= n {
			return evs
		}
		time.Sleep(5 * time.Millisecond)
	}
	return rec.list()
}

// assertNoEvent gives the (async) hook a chance to fire before declaring silence.
func assertNoEvent(t *testing.T, rec *disconnectRecorder, path string) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	if evs := rec.list(); len(evs) != 0 {
		t.Fatalf("%s is a CLEAN shutdown and must emit no disconnect event, got %+v", path, evs)
	}
}

// The positive case: a shared connection whose server dies reports the server,
// the error and how many calls were stranded, and is not marked scoped.
func TestPoolDisconnectFiresOnUnexpectedDeath(t *testing.T) {
	ctx := context.Background()
	p, rec, clients, cfg := poolWithDieableServer(t, "fake")

	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	cs := clients()
	if len(cs) != 1 {
		t.Fatalf("want 1 dialed client, got %d", len(cs))
	}
	cs[0].die(io.EOF, 3)

	evs := waitForEvents(t, rec, 1)
	if len(evs) != 1 {
		t.Fatalf("want 1 disconnect event, got %d (%+v)", len(evs), evs)
	}
	ev := evs[0]
	if ev.Server != "fake" {
		t.Errorf("server = %q, want fake", ev.Server)
	}
	if ev.Scoped || ev.ScopeKey != "" {
		t.Errorf("a shared connection must not be reported as scoped: %+v", ev)
	}
	if ev.PendingCalls != 3 {
		t.Errorf("pendingCalls = %d, want 3", ev.PendingCalls)
	}
	if ev.Error != io.EOF.Error() {
		t.Errorf("error = %q, want %q", ev.Error, io.EOF.Error())
	}
}

// A scoped connection's death names its scope, so the UI can say "this session
// lost the server" instead of "the server is down" workspace-wide.
func TestPoolDisconnectCarriesScope(t *testing.T) {
	ctx := context.Background()
	p, rec, clients, cfg := poolWithDieableServer(t, "fake")
	cfg.ScopeKey = "sess1|agent1"

	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	clients()[0].die(errors.New("boom"), 0)

	evs := waitForEvents(t, rec, 1)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %+v", evs)
	}
	ev := evs[0]
	if !ev.Scoped || ev.ScopeKey != "sess1|agent1" {
		t.Errorf("scope not carried: %+v", ev)
	}
	if ev.Server != "fake" {
		t.Errorf("server = %q, want fake (scope must be stripped)", ev.Server)
	}
	if got := ev.SessionID(); got != "sess1" {
		t.Errorf("SessionID() = %q, want sess1", got)
	}
}

// Clean path 1: the idle reaper evicting a scoped connection is routine
// housekeeping, not a server failure.
func TestPoolDisconnectSilentOnIdleReap(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	rec := &disconnectRecorder{}
	var mu sync.Mutex
	var clients []*fakeClient
	clock := func() time.Time { return now }
	p := newTestPool(time.Minute, func() time.Time { return clock() })
	t.Cleanup(p.Close)
	p.SetOnDisconnect(rec.record)
	cfg := dieableServer("fake", &clients, &mu)
	cfg.ScopeKey = "sess1|agent1"

	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	now = now.Add(2 * time.Minute) // push the entry past its idle window
	p.reapScoped()

	mu.Lock()
	c := clients[0]
	mu.Unlock()
	if c.Alive() {
		t.Fatal("reaper should have closed the connection")
	}
	assertNoEvent(t, rec, "idle reap (reapScoped)")
}

// Clean path 2: deleting a session closes its scoped connections on purpose.
func TestPoolDisconnectSilentOnCloseSession(t *testing.T) {
	ctx := context.Background()
	p, rec, clients, cfg := poolWithDieableServer(t, "fake")
	cfg.ScopeKey = "sess1|agent1"

	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	if closed := p.CloseSession("sess1"); closed != 1 {
		t.Fatalf("CloseSession closed %d connections, want 1", closed)
	}
	if clients()[0].Alive() {
		t.Fatal("CloseSession should have closed the connection")
	}
	assertNoEvent(t, rec, "CloseSession")
}

// Clean path 3: a changed server spec re-dials. The OLD connection is closed by
// us, so the swap must not look like the server crashed.
func TestPoolDisconnectSilentOnConfigChangeRedial(t *testing.T) {
	ctx := context.Background()
	p, rec, clients, cfg := poolWithDieableServer(t, "fake")

	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("first build errs: %v", errs)
	}
	changed := cfg
	changed.Args = []string{"--new-flag"} // different fingerprint => re-dial
	if _, _, errs := p.Catalog(ctx, []ServerConfig{changed}); len(errs) != 0 {
		t.Fatalf("second build errs: %v", errs)
	}

	cs := clients()
	if len(cs) != 2 {
		t.Fatalf("want a re-dial (2 clients), got %d", len(cs))
	}
	if cs[0].Alive() {
		t.Fatal("the superseded connection should be closed")
	}
	assertNoEvent(t, rec, "config-change re-dial")
}

// Clean path 4: Pool.Close is workspace teardown; every connection dies by our
// own hand and none of them is a disconnect event.
func TestPoolDisconnectSilentOnPoolClose(t *testing.T) {
	ctx := context.Background()
	rec := &disconnectRecorder{}
	var mu sync.Mutex
	var clients []*fakeClient
	p := newTestPool(scopedIdleTTL, nil)
	p.SetOnDisconnect(rec.record)
	shared := dieableServer("fake", &clients, &mu)
	scoped := shared
	scoped.ScopeKey = "sess1|agent1"

	if _, _, errs := p.Catalog(ctx, []ServerConfig{shared}); len(errs) != 0 {
		t.Fatalf("shared build errs: %v", errs)
	}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{scoped}); len(errs) != 0 {
		t.Fatalf("scoped build errs: %v", errs)
	}
	p.Close()

	mu.Lock()
	cs := append([]*fakeClient(nil), clients...)
	mu.Unlock()
	if len(cs) != 2 {
		t.Fatalf("want 2 connections before Close, got %d", len(cs))
	}
	for i, c := range cs {
		if c.Alive() {
			t.Fatalf("connection %d should be closed by Pool.Close", i)
		}
	}
	assertNoEvent(t, rec, "Pool.Close")
}

// splitEntryKey is what keeps a scoped death from being reported as a
// workspace-wide outage, so its two shapes are pinned directly.
func TestSplitEntryKey(t *testing.T) {
	if server, scope, scoped := splitEntryKey("fake"); server != "fake" || scope != "" || scoped {
		t.Errorf("shared key: got (%q,%q,%v)", server, scope, scoped)
	}
	key := scopedEntryKey("sess1|agent1", "fake")
	if server, scope, scoped := splitEntryKey(key); server != "fake" || scope != "sess1|agent1" || !scoped {
		t.Errorf("scoped key: got (%q,%q,%v)", server, scope, scoped)
	}
}
