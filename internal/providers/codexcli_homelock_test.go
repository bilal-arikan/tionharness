package providers

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAcquireCodexHomeIsExclusivePerHome(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()

	release, err := acquireCodexHome(context.Background(), dirA)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// A different home is an independent lock: serializing every codex turn in
	// the process regardless of home would be a needless throughput loss.
	releaseB, err := acquireCodexHome(context.Background(), dirB)
	if err != nil {
		t.Fatalf("acquire of an unrelated home blocked: %v", err)
	}
	releaseB()

	// The same home blocks until the holder releases.
	acquired := make(chan struct{})
	go func() {
		r, err := acquireCodexHome(context.Background(), dirA)
		if err != nil {
			t.Errorf("second acquire: %v", err)
			return
		}
		defer r()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("two turns held the same CODEX_HOME at once")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("release did not hand the home to the waiting turn")
	}
}

// An empty CODEX_HOME means the caller inherits the ambient ~/.codex and writes
// no config, so it must not queue behind unrelated turns.
func TestAcquireCodexHomeIgnoresEmptyDir(t *testing.T) {
	first, err := acquireCodexHome(context.Background(), "")
	if err != nil {
		t.Fatalf("acquire(\"\"): %v", err)
	}
	defer first()
	done := make(chan struct{})
	go func() {
		r, err := acquireCodexHome(context.Background(), "")
		if err != nil {
			t.Errorf("second acquire(\"\"): %v", err)
			return
		}
		r()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("empty dir serialized: turns with no config home must not block each other")
	}
}

// A turn cancelled while queued must give up rather than hold every later turn
// hostage until its predecessor's subprocess finishes.
func TestAcquireCodexHomeHonoursCancellationWhileWaiting(t *testing.T) {
	dir := t.TempDir()
	release, err := acquireCodexHome(context.Background(), dir)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := acquireCodexHome(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire error = %v, want context.Canceled", err)
	}
}

// Regression (SES431, TSK216): concurrent turns sharing a base home must run in
// separate shadow homes so their config.toml rewrites cannot clobber each other.
func TestConcurrentCodexTurnsDoNotClobberEachOthersConfig(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return // helper re-exec
	}
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}

	home := t.TempDir()
	t.Setenv("CODEX_TEST_HELPER_CONFIG", "enabled")
	t.Setenv("CODEX_TEST_HELPER_DELAY", "250ms")
	args := []string{"-test.run=TestCodexHelperFailsWhileUnityMCPConfigured", "-test.v=false"}

	// Separate providers, one shared home — exactly how PinCodexHome points
	// every codex agent at <dataDir>/codex-home.
	newTurn := func() *CodexCLI {
		return &CodexCLI{
			binPath:   self,
			configDir: home,
			model:     "gpt-test",
			mcpServers: map[string]CLIMCPServer{
				"unity-mcp": {Transport: "http", URL: "http://127.0.0.1:8080/mcp"},
			},
			mcpProbe: func(context.Context, string) bool { return true },
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := newTurn().completeWithArgs(context.Background(), args, "prompt", "gpt-test", Request{})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent shadow-home turn failed: %v", err)
		}
	}
}
