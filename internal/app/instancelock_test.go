package app

import (
	"errors"
	"testing"
)

// TestInstanceLockExclusive verifies the single-instance guard: while one lock is
// held on a data dir, a second acquire fails; after release, it can be re-taken.
// (A second acquire from the SAME process fails the same way a second process would
// — exclusive share mode on Windows, flock on Unix are both cross-open exclusive.)
func TestInstanceLockExclusive(t *testing.T) {
	dir := t.TempDir()

	l1, err := lockDataDir(dir)
	if err != nil {
		t.Fatalf("first lock should succeed: %v", err)
	}

	if _, err := lockDataDir(dir); err == nil {
		t.Fatal("second lock must fail while the first is held")
	} else if !errors.Is(err, errInstanceLocked) {
		t.Fatalf("second lock error should wrap errInstanceLocked, got: %v", err)
	}

	if err := l1.release(); err != nil {
		t.Fatalf("release should succeed: %v", err)
	}

	l2, err := lockDataDir(dir)
	if err != nil {
		t.Fatalf("re-lock after release should succeed: %v", err)
	}
	if err := l2.release(); err != nil {
		t.Fatalf("second release should succeed: %v", err)
	}

	// release must be idempotent (Shutdown may run after an error path already freed it).
	if err := l2.release(); err != nil {
		t.Fatalf("idempotent release should be a no-op, got: %v", err)
	}
}
