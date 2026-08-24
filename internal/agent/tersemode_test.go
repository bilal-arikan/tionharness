package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// TestTerseModeBlockFollowsToggle verifies the whole point of the feature: the
// terse prompt is UNCONDITIONAL when on (no use_skill call, no trigger word) and
// contributes zero bytes when off, so a workspace that never wants it pays
// nothing in its cached static prefix.
func TestTerseModeBlockFollowsToggle(t *testing.T) {
	r := &Runtime{}

	if got := r.TerseModeBlock(); got != "" {
		t.Errorf("terse mode defaults to OFF, got %q", got)
	}

	r.SetTerseMode(true)
	on := r.TerseModeBlock()
	if on == "" {
		t.Fatal("terse mode on but block is empty")
	}
	// With no workspace config dir, readPrompt resolves to the embedded default.
	if on != strings.TrimSpace(prompts.Default(TersePromptKey)) {
		t.Errorf("block is not the registry default:\n%s", on)
	}

	r.SetTerseMode(false)
	if got := r.TerseModeBlock(); got != "" {
		t.Errorf("terse mode off should send nothing, got %q", got)
	}
}

// TestSystemPromptAppendsTerseAfterInstructions pins the ORDER: terse comes after
// the workspace instructions, so a workspace rule can be phrased to override the
// reply style rather than being overridden by it.
func TestSystemPromptAppendsTerseAfterInstructions(t *testing.T) {
	agent := db.Agent{Soul: "You are Ada."}
	r := &Runtime{}
	r.SetInstructions("Always answer in Turkish.")
	r.SetTerseMode(true)

	got := r.systemPrompt(agent)
	insAt := strings.Index(got, "# Workspace Instructions")
	terseAt := strings.Index(got, "# Terse Mode")
	if insAt < 0 || terseAt < 0 {
		t.Fatalf("missing block(s): instructions=%d terse=%d\n%s", insAt, terseAt, got)
	}
	if terseAt < insAt {
		t.Errorf("terse block must come after workspace instructions:\n%s", got)
	}
	if !strings.Contains(got, "You are Ada.") {
		t.Errorf("persona dropped:\n%s", got)
	}
}
