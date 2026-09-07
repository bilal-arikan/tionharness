package db

import (
	"context"
	"testing"
)

func TestObserveTokenCalibrationRunsWindowedMean(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := CLIOverheadCalibrationKey("claude-cli", "2.1.220", "abc")

	c, err := d.ObserveTokenCalibration(ctx, TokenCalibration{Key: key, Kind: TokenCalibrationCLIOverhead, Tokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if c.Tokens != 100 || c.Samples != 1 {
		t.Fatalf("first sample = %d/%d, want 100/1", c.Tokens, c.Samples)
	}
	c, err = d.ObserveTokenCalibration(ctx, TokenCalibration{Key: key, Tokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	if c.Tokens != 200 || c.Samples != 2 {
		t.Fatalf("second sample = %d/%d, want 200/2", c.Tokens, c.Samples)
	}
	// Past the window the newest sample keeps at least 1/window of the weight:
	// 20 identical samples then one outlier moves the mean by outlier/window.
	for i := 0; i < 20; i++ {
		if _, err := d.ObserveTokenCalibration(ctx, TokenCalibration{Key: key, Tokens: 200}); err != nil {
			t.Fatal(err)
		}
	}
	c, err = d.ObserveTokenCalibration(ctx, TokenCalibration{Key: key, Tokens: 200 + 8*tokenCalibrationWindow})
	if err != nil {
		t.Fatal(err)
	}
	if c.Tokens != 208 {
		t.Fatalf("windowed mean = %d, want 208 (200 + 8*window/window)", c.Tokens)
	}
	if c.Samples != 23 {
		t.Fatalf("samples = %d, want 23", c.Samples)
	}
}

func TestSetTokenCalibrationReplacesAndSkipsUnchanged(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := PrefixCalibrationKey("anthropic", "claude-sonnet-5", "fp")
	if err := d.SetTokenCalibration(ctx, TokenCalibration{Key: key, Kind: TokenCalibrationPrefix, Tokens: 4200}); err != nil {
		t.Fatal(err)
	}
	c, ok := d.TokenCalibration(key)
	if !ok || c.Tokens != 4200 || c.Samples != 1 {
		t.Fatalf("stored = %+v ok=%v", c, ok)
	}
	first := c.UpdatedAt
	// Same value: no rewrite (timestamp untouched).
	if err := d.SetTokenCalibration(ctx, TokenCalibration{Key: key, Tokens: 4200}); err != nil {
		t.Fatal(err)
	}
	if c, _ = d.TokenCalibration(key); !c.UpdatedAt.Equal(first) {
		t.Fatal("unchanged exact count was rewritten")
	}
	// New value replaces, never averages.
	if err := d.SetTokenCalibration(ctx, TokenCalibration{Key: key, Tokens: 5000}); err != nil {
		t.Fatal(err)
	}
	if c, _ = d.TokenCalibration(key); c.Tokens != 5000 || c.Samples != 1 {
		t.Fatalf("replaced = %+v, want 5000/1", c)
	}
	if _, ok := d.TokenCalibration("missing"); ok {
		t.Fatal("missing key reported present")
	}
}

func TestTokenCalibrationSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := CLIOverheadCalibrationKey("claude-cli", "2.1.220", "abc")
	if _, err := d.ObserveTokenCalibration(ctx, TokenCalibration{Key: key, Kind: TokenCalibrationCLIOverhead, Provider: "claude-cli", Tokens: 26000}); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := again.TokenCalibration(key)
	if !ok || c.Tokens != 26000 || c.Samples != 1 || c.Provider != "claude-cli" {
		t.Fatalf("after reopen = %+v ok=%v", c, ok)
	}
	if n := len(again.TokenCalibrations(ctx)); n != 1 {
		t.Fatalf("calibrations after reopen = %d, want 1", n)
	}
}
