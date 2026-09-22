package decider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled || c.DefaultModel != "" {
		t.Errorf("defaults = %+v; the decider must start switched off", c)
	}
	for _, a := range Authorities() {
		ac, ok := c.Authorities[a.ID]
		if !ok || ac.Mode != a.DefaultMode || ac.Threshold != a.DefaultThreshold {
			t.Errorf("authority %s default = %+v", a.ID, ac)
		}
		// Every authority is off while the master switch is off.
		if c.Mode(a.ID) != ModeOff {
			t.Errorf("authority %s is %s with the master switch off", a.ID, c.Mode(a.ID))
		}
	}
}

func TestNormalizedClampsAndRepairs(t *testing.T) {
	c := Config{
		Enabled: true,
		Authorities: map[string]AuthorityConfig{
			testGate:     {Mode: ModeOn, Threshold: 0.2, Model: "DM2", Fallback: "DM2", Challenger: "DM3"},
			testExplicit: {Mode: ModeShadow}, // explicit authority: no shadow mode
			"retired":    {Mode: ModeOn},
		},
	}.Normalized()
	if got := c.Authorities[testGate]; got.Mode != ModeOn || got.Threshold != minThreshold || got.Fallback != "" || got.Challenger != "DM3" {
		t.Errorf("gate = %+v, want on, threshold clamped to %v, a fallback equal to the model dropped", got, minThreshold)
	}
	if c.Mode(testExplicit) != ModeOn {
		t.Errorf("explicit authority in shadow = %s, want on", c.Mode(testExplicit))
	}
	if _, ok := c.Authorities["retired"]; ok {
		t.Error("settings of an unregistered authority survived")
	}
	bogus := Config{Enabled: true, Authorities: map[string]AuthorityConfig{testGate: {Mode: "bogus"}}}.Normalized()
	if bogus.Mode(testGate) != ModeOff {
		t.Errorf("unknown mode = %s, want off", bogus.Mode(testGate))
	}
}

func TestConfigValidate(t *testing.T) {
	bad := []Config{
		{Authorities: map[string]AuthorityConfig{"mystery": {Mode: ModeOn}}},
		{Authorities: map[string]AuthorityConfig{testGate: {Mode: "loud"}}},
		{Authorities: map[string]AuthorityConfig{testGate: {Mode: ModeOn, Threshold: 1.5}}},
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

func TestWithoutMissingModels(t *testing.T) {
	c := Config{DefaultModel: "gone", Authorities: map[string]AuthorityConfig{
		testGate: {Model: "DM1", Fallback: "gone", Challenger: "gone"},
	}}
	known := func(id string) bool { return id == "DM1" }
	out := c.withoutMissingModels(known)
	if out.DefaultModel != "" || out.Authorities[testGate] != (AuthorityConfig{Model: "DM1"}) {
		t.Errorf("pruned = %+v", out)
	}
	if c.Authorities[testGate].Fallback != "gone" {
		t.Error("withoutMissingModels mutated its receiver")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if c, legacy, err := loadConfig(dir); err != nil || c.Enabled || legacy != nil {
		t.Fatalf("missing file: %+v, %+v, %v", c, legacy, err)
	}
	want := DefaultConfig()
	want.Enabled = true
	want.DefaultModel = "DM2"
	want.Authorities[testGate] = AuthorityConfig{Mode: ModeOn, Threshold: 0.75, Fallback: "DM1"}
	if err := saveConfig(dir, want); err != nil {
		t.Fatal(err)
	}
	got, legacy, err := loadConfig(dir)
	if err != nil || legacy != nil {
		t.Fatalf("load: %v legacy=%+v", err, legacy)
	}
	if !got.Enabled || got.DefaultModel != "DM2" || got.Authorities[testGate] != want.Authorities[testGate] {
		t.Errorf("round trip = %+v", got)
	}
	// A corrupt file is reported, never silently replaced.
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(dir); err == nil {
		t.Error("corrupt config loaded without error")
	}
}

func TestSeedModel(t *testing.T) {
	fresh := seedModel(nil)
	if fresh.Backend != OpenRouterBackendID || fresh.Model != JevModel || fresh.Credentials != CredentialsProvider || !fresh.Enabled {
		t.Errorf("fresh seed = %+v", fresh)
	}
	legacy := seedModel(&legacyConnection{Backend: "nope", ProviderInstanceID: "PRV7", Model: "typesafe/jev-2", TimeoutMs: 2000})
	if legacy.Backend != OpenRouterBackendID || legacy.ProviderInstanceID != "PRV7" || legacy.Model != "typesafe/jev-2" || legacy.TimeoutMs != 2000 {
		t.Errorf("legacy seed = %+v; an unknown backend keeps the default", legacy)
	}
}
