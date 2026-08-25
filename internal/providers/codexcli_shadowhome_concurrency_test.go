package providers

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

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
	barrier := t.TempDir()
	t.Setenv("CODEX_TEST_HELPER_CONFIG", "enabled")
	t.Setenv("CODEX_TEST_HELPER_BARRIER", barrier)
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

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := newTurn().completeWithArgs(ctx, args, "prompt", "gpt-test", Request{})
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
	entries, err := os.ReadDir(barrier)
	if err != nil {
		t.Fatalf("read concurrency barrier: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("turns did not overlap at the subprocess barrier: got %d distinct shadow homes, want 2", len(entries))
	}
	first, err := os.ReadFile(filepath.Join(barrier, entries[0].Name()))
	if err != nil {
		t.Fatalf("read first shadow home marker: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(barrier, entries[1].Name()))
	if err != nil {
		t.Fatalf("read second shadow home marker: %v", err)
	}
	if string(first) == string(second) {
		t.Fatalf("concurrent turns shared shadow home %q", first)
	}
}
