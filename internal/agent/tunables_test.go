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

func TestTunables_JournalDefaults(t *testing.T) {
	tun := NewTunables()
	if got := tun.JournalCap(); got != DefaultJournalCap {
		t.Errorf("JournalCap = %d, want default %d", got, DefaultJournalCap)
	}
	if got := tun.JournalMaxLen(); got != DefaultJournalMaxLen {
		t.Errorf("JournalMaxLen = %d, want default %d", got, DefaultJournalMaxLen)
	}
	// MinLen gate defaults off (0) until production wires it from settings.
	if got := tun.JournalMinLen(); got != 0 {
		t.Errorf("JournalMinLen = %d, want 0 (gate off) by default", got)
	}
	// Explicit values override; 0 falls back to the default for the caps, while a
	// minLen of 0 stays 0 (gate off — it is not a defaulted cap).
	tun.SetJournalLimits(7, 0, 0)
	if got := tun.JournalCap(); got != 7 {
		t.Errorf("JournalCap = %d, want 7", got)
	}
	if got := tun.JournalMaxLen(); got != DefaultJournalMaxLen {
		t.Errorf("JournalMaxLen = %d, want default after 0", got)
	}
	// A positive minLen is returned verbatim (gate on); a negative normalises to 0.
	tun.SetJournalLimits(7, 0, 64)
	if got := tun.JournalMinLen(); got != 64 {
		t.Errorf("JournalMinLen = %d, want 64", got)
	}
	tun.SetJournalLimits(7, 0, -5)
	if got := tun.JournalMinLen(); got != 0 {
		t.Errorf("JournalMinLen = %d, want 0 after negative", got)
	}
}

func TestTunables_RecallMinScore(t *testing.T) {
	tun := NewTunables()
	if got := tun.RecallMinScore(); got != DefaultRecallMinScore {
		t.Errorf("RecallMinScore = %v, want default %v", got, DefaultRecallMinScore)
	}
	tun.SetRecallMinScore(0.2)
	if got := tun.RecallMinScore(); got != 0.2 {
		t.Errorf("RecallMinScore = %v, want 0.2", got)
	}
	// 0 / negative fall back to the default (current behaviour).
	tun.SetRecallMinScore(0)
	if got := tun.RecallMinScore(); got != DefaultRecallMinScore {
		t.Errorf("RecallMinScore = %v, want default after 0", got)
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

// TestTunables_ReflectionCap verifies the reflection ring-buffer cap default and
// override (older reflections are pruned each dream cycle past this many).
func TestTunables_ReflectionCap(t *testing.T) {
	tun := NewTunables()
	if got := tun.ReflectionCap(); got != DefaultReflectionCap {
		t.Errorf("reflectionCap = %d, want default %d", got, DefaultReflectionCap)
	}
	tun.SetReflectionCap(7)
	if got := tun.ReflectionCap(); got != 7 {
		t.Errorf("reflectionCap = %d, want 7", got)
	}
	tun.SetReflectionCap(0) // 0 → default
	if got := tun.ReflectionCap(); got != DefaultReflectionCap {
		t.Errorf("reflectionCap after 0 = %d, want default %d", got, DefaultReflectionCap)
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
