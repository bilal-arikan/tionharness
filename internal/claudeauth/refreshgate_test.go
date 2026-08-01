package claudeauth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeCred writes a credential file with the given expiry (unix ms; 0 = wiped).
func writeCred(t *testing.T, home string, expiresAt int64, token string) {
	t.Helper()
	if err := WriteCredentials(home, Credential{
		AccessToken:  token,
		RefreshToken: token,
		ExpiresAt:    expiresAt,
		Scopes:       []string{"user:inference"},
	}); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}

// TestRefreshDue covers the classification the gate hangs off: only a token that is
// missing, wiped, or near expiry should serialize launches — otherwise a coordinator
// tree's whole point (parallel fan-out) would be throttled for nothing.
func TestRefreshDue(t *testing.T) {
	home := t.TempDir()
	if !refreshDue(home) {
		t.Error("a home with no credentials file must count as refresh-due")
	}

	// The exact shape the CLI leaves behind when a concurrent refresh fails.
	writeCred(t, home, 0, "")
	if !refreshDue(home) {
		t.Error("a wiped credential (empty token, expiresAt 0) must count as refresh-due")
	}

	writeCred(t, home, time.Now().Add(-time.Minute).UnixMilli(), "tok")
	if !refreshDue(home) {
		t.Error("an expired token must count as refresh-due")
	}

	// Inside the safety margin: still due, because the CLI refreshes proactively.
	writeCred(t, home, time.Now().Add(refreshMargin/2).UnixMilli(), "tok")
	if !refreshDue(home) {
		t.Error("a token expiring within the margin must count as refresh-due")
	}

	writeCred(t, home, time.Now().Add(3*time.Hour).UnixMilli(), "tok")
	if refreshDue(home) {
		t.Error("a comfortably valid token must NOT serialize launches")
	}
}

// TestSerializeRefreshIsNoopWhenTokenValid is the performance guarantee: with a
// healthy token, concurrent launches must not queue behind each other at all.
func TestSerializeRefreshIsNoopWhenTokenValid(t *testing.T) {
	home := t.TempDir()
	writeCred(t, home, time.Now().Add(3*time.Hour).UnixMilli(), "tok")

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); SerializeRefresh(home) }()
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("valid token must not serialize launches, took %v", elapsed)
	}
}

// TestSerializeRefreshAdmitsOneThenReleasesOnRefresh is the actual race fix: when a
// refresh is due, exactly one launcher is admitted, and the rest are released as
// soon as that launcher's refresh lands in the file — not after the watch window.
func TestSerializeRefreshAdmitsOneThenReleasesOnRefresh(t *testing.T) {
	home := t.TempDir()
	writeCred(t, home, time.Now().Add(-time.Minute).UnixMilli(), "old")

	// First launcher is admitted immediately.
	SerializeRefresh(home)

	// A second launcher must block while the first is presumed to be refreshing.
	second := make(chan struct{})
	go func() { SerializeRefresh(home); close(second) }()
	select {
	case <-second:
		t.Fatal("a second launcher was admitted while a refresh was in flight")
	case <-time.After(300 * time.Millisecond):
	}

	// Simulate the admitted launcher's refresh landing: the expiry advances.
	writeCred(t, home, time.Now().Add(3*time.Hour).UnixMilli(), "new")
	select {
	case <-second:
	case <-time.After(3 * time.Second):
		t.Fatal("the gate did not reopen after the refresh landed")
	}
}

// TestSerializeRefreshReleasesWithoutRefresh guards the liveness backstop: a launch
// that never refreshes (crashed early, older CLI) must not block its siblings
// forever — the watch window caps how long the gate stays closed.
func TestSerializeRefreshReleasesWithoutRefresh(t *testing.T) {
	// Shrink the backstop: the point is that it fires at all, not that it waits 30s.
	prev := refreshWatchWindow
	refreshWatchWindow = 600 * time.Millisecond
	defer func() { refreshWatchWindow = prev }()

	home := t.TempDir()
	writeCred(t, home, time.Now().Add(-time.Minute).UnixMilli(), "old")

	SerializeRefresh(home) // admitted; nothing will ever refresh
	done := make(chan struct{})
	go func() { SerializeRefresh(home); close(done) }()
	select {
	case <-done:
	case <-time.After(refreshWatchWindow + 5*time.Second):
		t.Fatal("the gate never reopened after the watch window elapsed")
	}
}

// TestSerializeRefreshEmptyHome: no isolated claude-home means nothing to
// serialize (the CLI runs against the ambient ~/.claude).
func TestSerializeRefreshEmptyHome(t *testing.T) {
	done := make(chan struct{})
	go func() { SerializeRefresh(""); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an empty home must return immediately")
	}
}

// TestExpiryOfUnreadable: a missing file reads as 0 rather than panicking, so the
// watcher treats "cannot read" as "no change observed yet".
func TestExpiryOfUnreadable(t *testing.T) {
	if got := expiryOf(filepath.Join(os.TempDir(), "tionswarm-no-such-home")); got != 0 {
		t.Errorf("expiryOf(missing) = %d, want 0", got)
	}
}
