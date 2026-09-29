package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentCatalogsRespectFirstFailureCooldown(t *testing.T) {
	var attempts atomic.Int32
	started := make(chan struct{}, 1)
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-finish
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "unavailable", server.URL)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { rt.WorkspaceToolCatalogWithState(t.Context()) })
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("catalog never dialed")
	}
	// Let the other builds reach the catalog while the first handshake is held.
	time.Sleep(100 * time.Millisecond)
	// A cached preview must not queue behind the pending handshake.
	preview := make(chan struct{})
	go func() { rt.WorkspaceToolCatalogWithState(WithCatalogNoDial(t.Context())); close(preview) }()
	select {
	case <-preview:
	case <-time.After(2 * time.Second):
		close(finish)
		wg.Wait()
		t.Fatal("cached preview blocked")
	}
	close(finish)
	wg.Wait()
	if got := attempts.Load(); got != 1 {
		t.Fatalf("concurrent outage dialed %d times, want 1", got)
	}
	if open, _, streak := rt.mcpFailStreaks.Open("unavailable"); !open || streak != 1 {
		t.Fatalf("breaker: open=%v streak=%d", open, streak)
	}
}

func TestCatalogGateWaitHonorsCancellation(t *testing.T) {
	var gate mcpCatalogGate
	release, err := gate.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := gate.acquire(ctx); err != context.Canceled {
		t.Fatalf("canceled waiter = %v", err)
	}
	release()
	release, err = gate.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestConcurrentCatalogsReuseHealthyConnection(t *testing.T) {
	server, attempts := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "ready", server.URL)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			defs, _ := rt.WorkspaceToolCatalogWithState(t.Context())
			for _, def := range defs {
				if def.Name == "ready__echo" {
					return
				}
			}
			t.Error("healthy MCP tool missing from concurrent catalog")
		})
	}
	wg.Wait()
	if attempts.Load() != 1 {
		t.Fatalf("healthy initialize calls = %d, want 1", attempts.Load())
	}
}
