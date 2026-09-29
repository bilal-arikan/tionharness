package exttools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVersionCacheSharesConcurrentProbes(t *testing.T) {
	var cache versionCache
	var calls atomic.Int32
	start := make(chan struct{})
	probe := func(context.Context) (string, error) { calls.Add(1); <-start; return "1.2.3", nil }
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			value, err := cache.get(t.Context(), "tool", probe)
			if err != nil || value != "1.2.3" {
				t.Errorf("probe = %q, %v", value, err)
			}
		})
	}
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("probes = %d, want 1", calls.Load())
	}
	cache.mu.Lock()
	cache.entries["tool"] = versionEntry{value: "old", expires: time.Now().Add(-time.Second)}
	cache.mu.Unlock()
	_, _ = cache.get(t.Context(), "tool", probe)
	if calls.Load() != 2 {
		t.Fatal("expired probe was reused")
	}
	cache.invalidate()
	_, _ = cache.get(t.Context(), "tool", probe)
	if calls.Load() != 3 {
		t.Fatal("explicit invalidation did not probe")
	}
}

func TestVersionCacheCancellationDoesNotCancelOtherWaiters(t *testing.T) {
	var cache versionCache
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		_, err := cache.get(ctx, "tool", func(work context.Context) (string, error) {
			close(started)
			<-finish
			return "1.0.0", work.Err()
		})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	close(finish)
	value, err := cache.get(t.Context(), "tool", func(context.Context) (string, error) { t.Error("duplicate probe"); return "", nil })
	if err != nil || value != "1.0.0" {
		t.Fatalf("other waiter = %q, %v", value, err)
	}
}

func TestVersionInvalidationDoesNotPublishOldInflightProbe(t *testing.T) {
	var cache versionCache
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.get(t.Context(), "tool", func(context.Context) (string, error) {
			close(started)
			<-finish
			return "1.0.0", nil
		})
	}()
	<-started
	cache.invalidate()
	_, _ = cache.get(t.Context(), "tool", func(context.Context) (string, error) { return "2.0.0", nil })
	close(finish)
	<-done
	value, _ := cache.get(t.Context(), "tool", func(context.Context) (string, error) { t.Error("unexpected probe"); return "", nil })
	if value != "2.0.0" {
		t.Fatalf("old probe replaced new version: %q", value)
	}
}

func TestVersionCacheRetriesErrorsAndKeysExecutableChanges(t *testing.T) {
	var cache versionCache
	_, err := cache.get(t.Context(), "tool", func(context.Context) (string, error) { return "", errors.New("failed") })
	if err == nil {
		t.Fatal("missing error")
	}
	value, err := cache.get(t.Context(), "tool", func(context.Context) (string, error) { return "2.0.0", nil })
	if err != nil || value != "2.0.0" {
		t.Fatalf("retry = %q, %v", value, err)
	}
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	key := versionProbeKey(path, []string{"--version"})
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if key == versionProbeKey(path, []string{"--version"}) {
		t.Fatal("replacement reused old key")
	}
	if key == versionProbeKey(path, []string{"-v"}) {
		t.Fatal("arguments must be part of the key")
	}
}
