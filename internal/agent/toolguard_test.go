package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func gcall(name, input string) providers.ToolCall {
	return providers.ToolCall{ID: "id-" + name, Name: name, Input: json.RawMessage(input)}
}

func failRes(id string) providers.ToolResult {
	return providers.ToolResult{CallID: id, Content: "boom", IsError: true}
}

func okRes(id string) providers.ToolResult {
	return providers.ToolResult{CallID: id, Content: "fine"}
}

func TestToolGuard_ExactFailureWarnsWithoutBlocking(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: true, hardStop: false})
	call := gcall("Read", `{"path":"x"}`)

	if hint := g.observe(call, failRes(call.ID)); hint != "" {
		t.Errorf("first failure hinted early: %q", hint)
	}
	hint := g.observe(call, failRes(call.ID))
	if hint == "" || !strings.Contains(hint, "failed 2 times") {
		t.Errorf("second identical failure must warn, got %q", hint)
	}
	// Warnings never block: check still allows with hard stop off, even far
	// past the block threshold.
	for i := 0; i < 10; i++ {
		g.observe(call, failRes(call.ID))
	}
	if v, _ := g.check(call); v != guardAllow {
		t.Errorf("hard stop off must always allow, got %v", v)
	}
}

func TestToolGuard_HardStopBlocksExactFailure(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: true, hardStop: true})
	call := gcall("Read", `{"path":"x"}`)
	for i := 0; i < DefaultGuardExactBlockAfter; i++ {
		g.observe(call, failRes(call.ID))
	}
	v, reason := g.check(call)
	if v != guardBlock {
		t.Fatalf("verdict = %v, want block after %d exact failures", v, DefaultGuardExactBlockAfter)
	}
	if reason == "" {
		t.Errorf("block verdict carries no reason")
	}
	// Different arguments are a fresh identity — allowed.
	if v, _ := g.check(gcall("Read", `{"path":"other"}`)); v != guardAllow {
		t.Errorf("different args must not inherit the block, got %v", v)
	}
}

func TestToolGuard_SameToolHalt(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: true, hardStop: true})
	// Same tool, different args every time: exact counter never accumulates,
	// the same-tool counter does.
	for i := 0; i < DefaultGuardSameToolHaltAfter; i++ {
		call := gcall("terminal", `{"cmd":"attempt-`+strings.Repeat("x", i)+`"}`)
		g.observe(call, failRes(call.ID))
	}
	v, _ := g.check(gcall("terminal", `{"cmd":"next"}`))
	if v != guardHalt {
		t.Fatalf("verdict = %v, want halt after %d same-tool failures", v, DefaultGuardSameToolHaltAfter)
	}
}

func TestToolGuard_SuccessResetsStreaks(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: true, hardStop: true})
	call := gcall("terminal", `{"cmd":"x"}`)
	for i := 0; i < 4; i++ {
		g.observe(call, failRes(call.ID))
	}
	g.observe(call, okRes(call.ID))
	if v, _ := g.check(call); v != guardAllow {
		t.Errorf("success must reset the failure streaks")
	}
	if g.sameFail["terminal"] != 0 {
		t.Errorf("sameFail = %d after success, want 0", g.sameFail["terminal"])
	}
}

func TestToolGuard_NoProgressOnIdempotentRepeats(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: true, hardStop: true})
	call := gcall("Read", `{"path":"same"}`) // Read is RiskRead → idempotent
	var hint string
	for i := 0; i < DefaultGuardNoProgressWarn+1; i++ {
		hint = g.observe(call, okRes(call.ID))
	}
	if hint == "" || !strings.Contains(hint, "not progress") {
		t.Errorf("identical successful repeats must warn, got %q", hint)
	}
	for i := 0; i < DefaultGuardNoProgressBlock; i++ {
		g.observe(call, okRes(call.ID))
	}
	if v, _ := g.check(call); v != guardBlock {
		t.Errorf("identical successful repeats past the block threshold must block")
	}
	// A mutating tool never triggers no-progress (side effects are progress).
	m := gcall("Write", `{"path":"same"}`)
	for i := 0; i < DefaultGuardNoProgressBlock+2; i++ {
		if hint := g.observe(m, okRes(m.ID)); hint != "" {
			t.Errorf("mutating tool repeat hinted: %q", hint)
		}
	}
	if v, _ := g.check(m); v != guardAllow {
		t.Errorf("mutating tool repeats must not block")
	}
}

func TestToolGuard_CustomThresholds(t *testing.T) {
	// Tightened thresholds from settings: warn at 1, block at 2, halt at 3.
	g := newToolGuard(toolGuardConfig{
		warnings: true, hardStop: true,
		exactWarn: 1, exactBlock: 2, sameToolWarn: 1, sameToolHalt: 3,
		noProgressWarn: 1, noProgressBlck: 2,
	})
	call := gcall("Read", `{"path":"x"}`)
	if hint := g.observe(call, failRes(call.ID)); hint == "" {
		t.Errorf("custom warn=1: first failure must already hint")
	}
	g.observe(call, failRes(call.ID))
	if v, _ := g.check(call); v != guardBlock {
		t.Errorf("custom block=2: verdict = %v, want block", v)
	}
	// Zero-value config resolves to the defaults (no premature warn).
	d := newToolGuard(toolGuardConfig{warnings: true})
	c2 := gcall("Grep", `{"q":"x"}`)
	if hint := d.observe(c2, failRes(c2.ID)); hint != "" {
		t.Errorf("default thresholds: first failure hinted early: %q", hint)
	}
	if d.cfg.exactBlock != DefaultGuardExactBlockAfter || d.cfg.sameToolHalt != DefaultGuardSameToolHaltAfter {
		t.Errorf("zero config not resolved to defaults: %+v", d.cfg)
	}
}

func TestToolGuard_WarningsDisabledStaysSilent(t *testing.T) {
	g := newToolGuard(toolGuardConfig{warnings: false, hardStop: false})
	call := gcall("Read", `{"path":"x"}`)
	for i := 0; i < 6; i++ {
		if hint := g.observe(call, failRes(call.ID)); hint != "" {
			t.Errorf("warnings off must never hint, got %q", hint)
		}
	}
}
