package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// seedHook creates an enabled tool hook for the detection tests.
func seedHook(t *testing.T, r *Runtime, event, matcher, command string) {
	t.Helper()
	if _, err := r.db.CreateHook(context.Background(), db.Hook{
		Event: event, Type: "command", Matcher: matcher, Command: command, Enabled: true, TimeoutSec: 30,
	}); err != nil {
		t.Fatalf("seed hook: %v", err)
	}
}

func TestDetectTokenOptimizers_None(t *testing.T) {
	r := lifecycleRuntime(t)
	if st := r.detectTokenOptimizers(context.Background()); st.present() {
		t.Fatalf("expected no optimizers, got %+v", st)
	}
}

// Mirrors the real WS5 wiring: HOK1 "sqz hook claude" + HOK2 rtk auto-wrap, both
// PreToolUse / matcher Bash.
func TestDetectTokenOptimizers_WS5Shape(t *testing.T) {
	r := lifecycleRuntime(t)
	seedHook(t, r, db.HookPreToolUse, "Bash", "sqz hook claude")
	seedHook(t, r, db.HookPreToolUse, "Bash",
		`$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c }`)

	st := r.detectTokenOptimizers(context.Background())
	if !st.rtk || !st.sqz {
		t.Fatalf("expected both rtk and sqz, got %+v", st)
	}
	if len(st.matchers) != 1 || st.matchers[0] != "Bash" {
		t.Fatalf("expected matcher [Bash], got %v", st.matchers)
	}

	block := tokenOptimizerGuidance(st)
	for _, want := range []string{"rtk", "sqz", "Token optimization active", "`Bash`", "ONLY"} {
		if !strings.Contains(block, want) {
			t.Errorf("guidance missing %q:\n%s", want, block)
		}
	}
}

func TestDetectTokenOptimizers_MarkerIsWordBounded(t *testing.T) {
	r := lifecycleRuntime(t)
	// "quirtky" and "sqzip" must NOT trigger detection (substring false positives).
	seedHook(t, r, db.HookPostToolUse, "Bash", "echo quirtky && sqzip --help")
	if st := r.detectTokenOptimizers(context.Background()); st.present() {
		t.Fatalf("word-bounded markers must not match substrings, got %+v", st)
	}
}

// A wildcard matcher suppresses the scope note (everything is covered already).
func TestTokenOptimizerGuidance_WildcardNoScopeNote(t *testing.T) {
	block := tokenOptimizerGuidance(tokenOptimizerState{sqz: true, matchers: []string{"*"}})
	if strings.Contains(block, "ONLY") {
		t.Errorf("wildcard matcher must not emit a scope note:\n%s", block)
	}
}

// A workspace with ShellOutputCompression="on" compresses shell output through
// the in-process filter with no hook wired. The agent must still be told, or it
// reads sqz's abbreviated output as truncation. Regression for the gap where the
// capability consulted hooks only.
func TestEffectiveTokenOptimizers_ForcedOnWithoutHook(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()

	if st := r.effectiveTokenOptimizers(ctx); st.present() {
		t.Fatalf("no hook and auto mode: expected nothing, got %+v", st)
	}

	r.SetShellCompression("on")
	st := r.effectiveTokenOptimizers(ctx)
	if !st.sqz {
		t.Fatalf("forced on: expected sqz reported, got %+v", st)
	}
	if st.rtk {
		t.Fatalf("forced on must not claim rtk, got %+v", st)
	}
	block := tokenOptimizerGuidance(st)
	if !strings.Contains(block, "sqz") {
		t.Fatalf("guidance missing sqz: %q", block)
	}
	// The in-process filter covers every shell call, so no narrowing scope note.
	if strings.Contains(block, "ONLY") {
		t.Fatalf("forced on must not narrow scope: %q", block)
	}
}

// "off" disables the in-process filter but must not erase a real sqz hook, which
// still fires on the native tool name.
func TestEffectiveTokenOptimizers_OffKeepsHook(t *testing.T) {
	r := lifecycleRuntime(t)
	seedHook(t, r, db.HookPreToolUse, "Bash", "sqz hook claude")
	r.SetShellCompression("off")

	if st := r.effectiveTokenOptimizers(context.Background()); !st.sqz {
		t.Fatalf("off must not hide a wired sqz hook, got %+v", st)
	}
}
