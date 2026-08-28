package agent

import (
	"sync"
	"testing"
)

func TestTunables_Defaults(t *testing.T) {
	tun := NewTunables()
	if tun.MaxOutputTokens() != 0 {
		t.Errorf("MaxOutputTokens = %d, want 0", tun.MaxOutputTokens())
	}
	if tun.ShellEnabled() {
		t.Error("ShellEnabled = true, want false")
	}
}

func TestTunables_SetAndGet(t *testing.T) {
	tun := NewTunables()

	tun.SetMaxOutputTokens(4096)
	if tun.MaxOutputTokens() != 4096 {
		t.Errorf("MaxOutputTokens = %d, want 4096", tun.MaxOutputTokens())
	}
}

// TestTunables_ConcurrentAccess exercises the RWMutex under the race detector.
func TestTunables_ConcurrentAccess(t *testing.T) {
	tun := NewTunables()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); tun.SetShellEnabled(true); _ = tun.ShellEnabled() }()
		go func() { defer wg.Done(); tun.SetMaxOutputTokens(1000); _ = tun.MaxOutputTokens() }()
	}
	wg.Wait()
}
