package decider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled {
		t.Error("the decider must start switched off")
	}
	if c.Backend != OpenRouterBackendID || c.Model != JevModel || c.TimeoutMs != 3000 {
		t.Errorf("defaults = %+v", c)
	}
	for _, s := range Sites() {
		sc, ok := c.Sites[s.ID]
		if !ok || sc.Mode != s.DefaultMode || sc.Threshold != s.DefaultThreshold {
			t.Errorf("site %s default = %+v", s.ID, sc)
		}
		// Every site is off while the master switch is off.
		if c.SiteMode(s.ID) != ModeOff {
			t.Errorf("site %s is %s with the master switch off", s.ID, c.SiteMode(s.ID))
		}
	}
}

func TestNormalizedClampsAndRepairs(t *testing.T) {
	c := Config{
		Enabled:   true,
		TimeoutMs: 60000,
		Sites: map[string]SiteConfig{
			SiteToolRisk:  {Mode: ModeOn, Threshold: 0.2},
			SiteFlowJudge: {Mode: ModeShadow}, // explicit site: no shadow mode
			SiteStallJudge: {
				Mode: "bogus",
			},
		},
	}.Normalized()
	if c.TimeoutMs != maxTimeoutMs {
		t.Errorf("timeout = %d, want clamped to %d", c.TimeoutMs, maxTimeoutMs)
	}
	if got := c.Sites[SiteToolRisk]; got.Mode != ModeOn || got.Threshold != minThreshold {
		t.Errorf("tool-risk = %+v, want on with threshold clamped to %v", got, minThreshold)
	}
	if c.SiteMode(SiteFlowJudge) != ModeOn {
		t.Errorf("explicit site in shadow = %s, want on", c.SiteMode(SiteFlowJudge))
	}
	if c.SiteMode(SiteStallJudge) != ModeOff {
		t.Errorf("unknown mode = %s, want off", c.SiteMode(SiteStallJudge))
	}
	if c.Backend != OpenRouterBackendID || c.Model != JevModel {
		t.Errorf("backend/model defaults not filled: %+v", c)
	}
}

func TestConfigValidate(t *testing.T) {
	bad := []Config{
		{Backend: "nope"},
		{Sites: map[string]SiteConfig{"mystery": {Mode: ModeOn}}},
		{Sites: map[string]SiteConfig{SiteToolRisk: {Mode: "loud"}}},
		{Sites: map[string]SiteConfig{SiteToolRisk: {Mode: ModeOn, Threshold: 1.5}}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: invalid config accepted: %+v", i, c)
		}
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("default config rejected: %v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if c, err := LoadConfig(dir); err != nil || c.Enabled {
		t.Fatalf("missing file: %+v, %v", c, err)
	}
	want := DefaultConfig()
	want.Enabled = true
	want.ProviderInstanceID = "PRV3"
	want.Sites[SiteStallJudge] = SiteConfig{Mode: ModeOn, Threshold: 0.75}
	if err := SaveConfig(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.ProviderInstanceID != "PRV3" || got.Sites[SiteStallJudge].Threshold != 0.75 {
		t.Errorf("round trip = %+v", got)
	}
	// A corrupt file is reported, never silently replaced.
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Error("corrupt config loaded without error")
	}
}
