package ingest

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/market"
)

// TestHookAdapterScansPluginHooks verifies the hook adapter discovers a Claude
// Code plugin.json hooks block, emits one KindHook pack per command, bundles the
// referenced script, and preserves the ${CLAUDE_PLUGIN_ROOT} placeholder.
func TestHookAdapterScansPluginHooks(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude-plugin/plugin.json", `{
      "name": "caveman",
      "hooks": {
        "SessionStart": [
          { "hooks": [ { "type": "command", "command": "node ${CLAUDE_PLUGIN_ROOT}/src/hooks/caveman-activate.js", "timeout": 5 } ] }
        ],
        "UserPromptSubmit": [
          { "hooks": [ { "type": "command", "command": "node ${CLAUDE_PLUGIN_ROOT}/src/hooks/caveman-mode-tracker.js" } ] }
        ]
      }
    }`)
	write(t, root, "src/hooks/caveman-activate.js", "// activate")
	write(t, root, "src/hooks/caveman-mode-tracker.js", "// tracker")

	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}

	var hooks []Discovered
	for _, it := range sr.Items {
		if it.Kind == market.KindHook {
			hooks = append(hooks, it)
		}
	}
	if len(hooks) != 2 {
		t.Fatalf("want 2 hook items, got %d (%+v)", len(hooks), sr.Items)
	}

	// Build the SessionStart hook and check its payload + bundled script.
	var ss *Discovered
	for i := range hooks {
		if strings.HasPrefix(hooks[i].Name, "SessionStart") {
			ss = &hooks[i]
		}
	}
	if ss == nil {
		t.Fatalf("SessionStart hook not discovered: %+v", hooks)
	}
	packs, _, _, err := BuildPacks("local", root, []string{ss.Key}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 {
		t.Fatalf("want 1 built pack, got %d", len(packs))
	}
	hp := packs[0].Payload.Hook
	if hp == nil {
		t.Fatal("built pack has no hook payload")
	}
	if hp.Event != "SessionStart" {
		t.Errorf("event: want SessionStart, got %q", hp.Event)
	}
	if !strings.Contains(hp.Command, "${CLAUDE_PLUGIN_ROOT}") {
		t.Errorf("command should keep the placeholder for the installer to rewrite: %q", hp.Command)
	}
	if hp.TimeoutSec != 5 {
		t.Errorf("timeout: want 5, got %d", hp.TimeoutSec)
	}
	if _, ok := packs[0].Files["src/hooks/caveman-activate.js"]; !ok {
		t.Errorf("bundled script missing; files: %v", keysOf(packs[0].Files))
	}
}

// TestHookAdapterIgnoresNonHookJSON ensures a plain settings.json with no hooks
// block produces no hook packs (no false positives).
func TestHookAdapterIgnoresNonHookJSON(t *testing.T) {
	root := t.TempDir()
	write(t, root, "settings.json", `{"theme":"dark","fontSize":14}`)
	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range sr.Items {
		if it.Kind == market.KindHook {
			t.Fatalf("unexpected hook discovered from a non-hook settings.json: %+v", it)
		}
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
