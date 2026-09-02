package mcp

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A server whose handshake never answers must not hold the catalog build for as
// long as the caller's context lives: the pool bounds every (re)dial with its
// own deadline, so the build returns with that server recorded as failed.
func TestPoolCatalogBoundsHungDial(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)
	p.SetDialTimeout(60 * time.Millisecond)

	cfg := ServerConfig{
		Name:    "hang",
		Command: "never-runs",
		stdioDial: func(ctx context.Context, _ string, _, _ []string, _ string) (Client, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	start := time.Now()
	entries, _, errs := p.Catalog(context.Background(), []ServerConfig{cfg})
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("catalog took %s, want the dial deadline to cut it short", took)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want none from a hung server", len(entries))
	}
	if msg := errs["hang"]; !strings.Contains(msg, "deadline") {
		t.Fatalf("errs[hang] = %q, want a deadline error", msg)
	}
}

// A zero timeout disables the bound (the caller's context is the only limit).
func TestPoolDialTimeoutZeroMeansUnbounded(t *testing.T) {
	p := NewPool()
	t.Cleanup(p.Close)
	p.SetDialTimeout(0)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	cfg := ServerConfig{
		Name:    "hang",
		Command: "never-runs",
		stdioDial: func(ctx context.Context, _ string, _, _ []string, _ string) (Client, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	_, _, errs := p.Catalog(ctx, []ServerConfig{cfg})
	if msg := errs["hang"]; !strings.Contains(msg, "deadline") {
		t.Fatalf("errs[hang] = %q, want the caller's deadline error", msg)
	}
}
