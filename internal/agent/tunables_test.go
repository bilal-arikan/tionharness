package agent

import (
	"sync"
	"testing"
)

func TestTunables_Defaults(t *testing.T) {
	tun := NewTunables()
	if tun.TitleModel() != "" {
		t.Errorf("TitleModel = %q, want empty", tun.TitleModel())
	}
}

func TestTunables_SetAndGet(t *testing.T) {
	tun := NewTunables()

	tun.SetTitleModel("haiku")
	if tun.TitleModel() != "haiku" {
		t.Errorf("TitleModel = %q, want haiku", tun.TitleModel())
	}
}

// TestTunables_ConcurrentAccess exercises the RWMutex under the race detector.
func TestTunables_ConcurrentAccess(t *testing.T) {
	tun := NewTunables()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); tun.SetTitleModel("m"); _ = tun.TitleModel() }()
		go func() { defer wg.Done(); tun.SetContextBudget(1000); _ = tun.TitleModel() }()
	}
	wg.Wait()
}
