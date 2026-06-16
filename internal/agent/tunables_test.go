package agent

import (
	"sync"
	"testing"
)

func TestTunables_Defaults(t *testing.T) {
	tun := NewTunables()
	if tun.AutonomyPaused() {
		t.Error("new Tunables should not be paused")
	}
	if tun.TitleModel() != "" {
		t.Errorf("TitleModel = %q, want empty", tun.TitleModel())
	}
}

func TestTunables_SetAndGet(t *testing.T) {
	tun := NewTunables()

	tun.SetAutonomyPaused(true)
	if !tun.AutonomyPaused() {
		t.Error("AutonomyPaused should be true after SetAutonomyPaused(true)")
	}
	tun.SetAutonomyPaused(false)
	if tun.AutonomyPaused() {
		t.Error("AutonomyPaused should be false after SetAutonomyPaused(false)")
	}

	tun.SetTitleModel("haiku")
	if tun.TitleModel() != "haiku" {
		t.Errorf("TitleModel = %q, want haiku", tun.TitleModel())
	}
}

func TestTunables_JournalDefaults(t *testing.T) {
	tun := NewTunables()
	if got := tun.JournalCap(); got != DefaultJournalCap {
		t.Errorf("JournalCap = %d, want default %d", got, DefaultJournalCap)
	}
	if got := tun.JournalMaxLen(); got != DefaultJournalMaxLen {
		t.Errorf("JournalMaxLen = %d, want default %d", got, DefaultJournalMaxLen)
	}
	// Explicit values override; 0 falls back to the default.
	tun.SetJournalLimits(7, 0)
	if got := tun.JournalCap(); got != 7 {
		t.Errorf("JournalCap = %d, want 7", got)
	}
	if got := tun.JournalMaxLen(); got != DefaultJournalMaxLen {
		t.Errorf("JournalMaxLen = %d, want default after 0", got)
	}
}

func TestTunables_AutoReflect(t *testing.T) {
	tun := NewTunables()
	if tun.AutoReflect() {
		t.Error("auto-reflect should default off until configured")
	}
	if got := tun.AutoReflectThreshold(); got != DefaultAutoReflectThreshold {
		t.Errorf("threshold = %d, want default %d", got, DefaultAutoReflectThreshold)
	}
	tun.SetAutoReflect(true, 12)
	if !tun.AutoReflect() {
		t.Error("auto-reflect should be on after SetAutoReflect(true, …)")
	}
	if got := tun.AutoReflectThreshold(); got != 12 {
		t.Errorf("threshold = %d, want 12", got)
	}
}

// TestTunables_ConcurrentAccess exercises the RWMutex under the race detector.
func TestTunables_ConcurrentAccess(t *testing.T) {
	tun := NewTunables()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); tun.SetAutonomyPaused(true); _ = tun.AutonomyPaused() }()
		go func() { defer wg.Done(); tun.SetTitleModel("m"); _ = tun.TitleModel() }()
	}
	wg.Wait()
}
