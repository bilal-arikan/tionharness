package tools

import (
	"context"
	"testing"
)

// TestIsRTKWrapped: only the PROGRAM being invoked counts. Matching "rtk" anywhere
// in the line would tag ordinary commands that merely mention it (a grep, a path)
// with an optimizer chip they never went through.
func TestIsRTKWrapped(t *testing.T) {
	wrapped := []string{
		"rtk git status",
		"  rtk   cargo test  ",
		"RTK.exe npm build",
	}
	for _, cmd := range wrapped {
		if !isRTKWrapped(cmd) {
			t.Errorf("expected %q to be recognised as rtk-wrapped", cmd)
		}
	}

	plain := []string{
		"",
		"   ",
		"grep rtk build.log",
		"cat /c/Users/user/Desktop/Progs/rtk/README.md",
		"git status", // the very command rtk would wrap, run bare
		"rtkfoo bar", // a different program that merely starts with the letters
	}
	for _, cmd := range plain {
		if isRTKWrapped(cmd) {
			t.Errorf("expected %q NOT to be recognised as rtk-wrapped", cmd)
		}
	}
}

// TestShellOptimizationMeasured guards the honesty rule: a chip may only claim a
// percentage when there is a real before/after pair behind it.
func TestShellOptimizationMeasured(t *testing.T) {
	measured := ShellOptimization{Kind: "sqz", InTokens: 28, OutTokens: 8}
	if !measured.Measured() {
		t.Fatal("28 → 8 tokens must count as measured")
	}
	if got := measured.Percent(); got != 71 {
		t.Errorf("Percent() = %d, want 71", got)
	}

	unmeasured := []ShellOptimization{
		{Kind: "rtk"}, // rtk shrinks upstream; nothing to compare
		{Kind: "sqz", InTokens: 100, OutTokens: 100}, // untouched
		{Kind: "sqz", InTokens: 100, OutTokens: 120}, // grew
		{Kind: "sqz", InTokens: 0, OutTokens: 8},     // missing input count
		{Kind: "sqz", Dedup: true},                   // dedup reports no pair
	}
	for _, o := range unmeasured {
		if o.Measured() {
			t.Errorf("%+v must not count as measured", o)
		}
		if o.Percent() != 0 {
			t.Errorf("%+v must report 0%%, got %d", o, o.Percent())
		}
	}
}

// TestOptimizerSink covers the native-loop seam: one record per tool call, drained
// by Take, and a nil-safe no-op when no sink is attached (autonomous runs).
func TestOptimizerSink(t *testing.T) {
	ctx, sink := WithOptimizerSink(context.Background())
	if sink.Take() != nil {
		t.Fatal("a fresh sink must be empty")
	}

	recordOptimization(ctx, ShellOptimization{Kind: "rtk"})
	// A later record wins, so a command that is BOTH rtk-wrapped and sqz-compressed
	// shows the measured sqz figure rather than the bare rtk name.
	recordOptimization(ctx, ShellOptimization{Kind: "sqz", InTokens: 28, OutTokens: 8})

	got := sink.Take()
	if got == nil || got.Kind != "sqz" || got.Percent() != 71 {
		t.Fatalf("expected the later sqz record to win, got %+v", got)
	}
	if sink.Take() != nil {
		t.Fatal("Take must clear the sink so the next call starts empty")
	}

	// No sink on ctx: recording must not panic (autonomous runs render no trace).
	recordOptimization(context.Background(), ShellOptimization{Kind: "sqz"})

	var nilSink *optimizerSink
	if nilSink.Take() != nil {
		t.Fatal("Take on a nil sink must be safe")
	}
}
