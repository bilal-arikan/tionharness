package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClient is a live-enough Client for pool tests: it lists a fixed tool set
// and reports itself connected until Close.
type fakeClient struct {
	mu     sync.Mutex
	tools  []Tool
	closed bool
	// onDead is the pool's disconnect hook. It mirrors StdioClient: the fake
	// fires it only from die(), never from Close(), so a test that closes the
	// client the way the pool does cannot produce a disconnect event.
	onDead func(err error, pendingCalls int)
}

func (c *fakeClient) ListTools(context.Context) ([]Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools, nil
}

func (c *fakeClient) CallTool(context.Context, string, json.RawMessage) (CallToolResult, error) {
	return CallToolResult{Text: "ok"}, nil
}

func (c *fakeClient) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

func (c *fakeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeClient) SetOnToolsChanged(func())       {}
func (c *fakeClient) SetLogger(*slog.Logger, string) {}

func (c *fakeClient) SetOnDisconnect(fn func(err error, pendingCalls int)) {
	c.mu.Lock()
	c.onDead = fn
	c.mu.Unlock()
}

// die simulates the server process dying under a live connection: the client
// goes not-Alive and the disconnect hook fires, exactly as StdioClient.failAll
// does for a read loop that exited without a preceding Close.
func (c *fakeClient) die(err error, pending int) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return // already torn down by us; a real client stays quiet here too
	}
	c.closed = true
	fn := c.onDead
	c.mu.Unlock()
	if fn != nil {
		fn(err, pending)
	}
}

// hangingServer is a config whose dial never answers, so it can only end at the
// dial deadline.
func hangingServer(name string) ServerConfig {
	return ServerConfig{
		Name:    name,
		Command: "never-runs",
		stdioDial: func(ctx context.Context, _ string, _, _ []string, _ string) (Client, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
}

// liveServer is a config that dials instantly and advertises the given tools.
func liveServer(name string, toolNames ...string) ServerConfig {
	ts := make([]Tool, 0, len(toolNames))
	for _, n := range toolNames {
		ts = append(ts, Tool{Name: n, Description: n + " description"})
	}
	return ServerConfig{
		Name:    name,
		Command: "runs",
		stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			return &fakeClient{tools: ts}, nil
		},
	}
}

// The whole point of EnsureServers over a Catalog loop: N dead servers cost ONE
// dial deadline, not N of them. Serially, five hung servers at the 200ms deadline
// used here would be ~1s; in parallel it must finish near a single deadline.
func TestEnsureServersDialsInParallel(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)
	const deadline = 200 * time.Millisecond
	p.SetDialTimeout(deadline)

	cfgs := []ServerConfig{
		hangingServer("hang1"), hangingServer("hang2"), hangingServer("hang3"),
		hangingServer("hang4"), hangingServer("hang5"),
	}
	start := time.Now()
	out := p.EnsureServers(context.Background(), cfgs)
	took := time.Since(start)

	// Serial would be 5 x deadline; allow generous slack for a loaded CI box but
	// stay well under the serial cost.
	if took >= 3*deadline {
		t.Fatalf("EnsureServers took %s for 5 hung servers (deadline %s); dials are not parallel", took, deadline)
	}
	if len(out) != len(cfgs) {
		t.Fatalf("got %d outcomes, want %d", len(out), len(cfgs))
	}
	for i, o := range out {
		if o.Server != cfgs[i].Name {
			t.Errorf("outcome %d is for %q, want %q (order must match the input)", i, o.Server, cfgs[i].Name)
		}
		if o.State != ServerDead {
			t.Errorf("%s: state = %v, want ServerDead", o.Server, o.State)
		}
		if !strings.Contains(o.Err, "deadline") {
			t.Errorf("%s: err = %q, want a deadline error", o.Server, o.Err)
		}
	}
}

// A mixed set reports each server independently: one dead server must not erase
// the news that the others are up, and the live ones' tools come back.
func TestEnsureServersReportsPerServerOutcome(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)
	p.SetDialTimeout(100 * time.Millisecond)

	out := p.EnsureServers(context.Background(), []ServerConfig{
		liveServer("alpha", "one", "two"),
		hangingServer("broken"),
		liveServer("beta", "three"),
	})

	if len(out) != 3 {
		t.Fatalf("got %d outcomes, want 3", len(out))
	}
	if out[0].State != ServerAlive || len(out[0].Tools) != 2 {
		t.Errorf("alpha = %+v, want alive with 2 tools", out[0])
	}
	if out[1].State != ServerDead || out[1].Err == "" {
		t.Errorf("broken = %+v, want dead with a reason", out[1])
	}
	if out[2].State != ServerAlive || len(out[2].Tools) != 1 {
		t.Errorf("beta = %+v, want alive with 1 tool", out[2])
	}

	entries := CatalogEntries(out)
	if len(entries) != 3 {
		t.Fatalf("CatalogEntries returned %d entries, want 3 (dead servers contribute none)", len(entries))
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.NamespacedName] = true
	}
	for _, want := range []string{"alpha__one", "alpha__two", "beta__three"} {
		if !got[want] {
			t.Errorf("missing catalog entry %q (got %v)", want, got)
		}
	}
}

// A warmed connection must be the SAME pool slot a later Catalog reuses —
// otherwise this is a probe, not a warm-up, and the tools would not actually be
// reachable. Dialing once and then cataloguing must not re-dial.
func TestEnsureServersWarmsTheSlotCatalogReuses(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)

	var dials int
	var mu sync.Mutex
	cfg := ServerConfig{
		Name:    "warm",
		Command: "runs",
		stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			mu.Lock()
			dials++
			mu.Unlock()
			return &fakeClient{tools: []Tool{{Name: "t"}}}, nil
		},
	}

	if out := p.EnsureServers(context.Background(), []ServerConfig{cfg}); out[0].State != ServerAlive {
		t.Fatalf("warm-up failed: %+v", out[0])
	}
	if got := p.ServerState("warm"); got != ServerAlive {
		t.Fatalf("ServerState = %v after warm-up, want ServerAlive", got)
	}
	entries, _, errs := p.Catalog(context.Background(), []ServerConfig{cfg})
	if len(errs) != 0 {
		t.Fatalf("catalog errs = %v, want none", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("catalog entries = %d, want 1", len(entries))
	}
	mu.Lock()
	defer mu.Unlock()
	if dials != 1 {
		t.Fatalf("dials = %d, want 1 — Catalog must reuse the slot EnsureServers warmed", dials)
	}
}

// A scoped server gets its own per-caller slot, exactly as Catalog/Call route it,
// so warming under a scope key cannot leave the shared slot connected instead.
func TestEnsureServersHonoursScopeKey(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)

	cfg := liveServer("scoped", "t")
	cfg.ScopeKey = "sess1|agentA"
	if out := p.EnsureServers(context.Background(), []ServerConfig{cfg}); out[0].State != ServerAlive {
		t.Fatalf("warm-up failed: %+v", out[0])
	}

	var scopedSlots, sharedSlots int
	for _, s := range p.Stats() {
		if s.Server != "scoped" {
			continue
		}
		if s.Scoped {
			scopedSlots++
			if s.ScopeKey != "sess1|agentA" {
				t.Errorf("scope key = %q, want sess1|agentA", s.ScopeKey)
			}
		} else {
			sharedSlots++
		}
	}
	if scopedSlots != 1 {
		t.Errorf("scoped slots = %d, want 1", scopedSlots)
	}
	if sharedSlots != 0 {
		t.Errorf("shared slots = %d, want 0 — a scoped warm-up must not touch the shared slot", sharedSlots)
	}
}
