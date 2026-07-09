package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readSettings parses a claude-home settings.json into a generic map.
func readSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return cfg
}

// TestEnsureClaudeHomeEffortLevel: the batching guard (Claude Code ≥2.1.203
// serialises parallel tool calls when effortLevel is unset) must land an
// explicit effortLevel in a workspace claude-home — creating the file, merging
// into an existing one without clobbering other keys, never overwriting an
// explicit value, and leaving an unparseable file untouched.
func TestEnsureClaudeHomeEffortLevel(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, "settings.json")

	// No settings.json yet → created with a non-empty effortLevel.
	ensureClaudeHomeEffortLevel(home)
	if v, _ := readSettings(t, home)["effortLevel"].(string); v == "" {
		t.Fatal("fresh home must gain a non-empty effortLevel")
	}

	// Existing file WITHOUT the key → key merged, other keys preserved.
	if err := os.WriteFile(settings, []byte(`{"model":"opus","theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureClaudeHomeEffortLevel(home)
	cfg := readSettings(t, home)
	if v, _ := cfg["effortLevel"].(string); v == "" {
		t.Error("missing effortLevel must be added to an existing settings file")
	}
	if cfg["model"] != "opus" || cfg["theme"] != "dark" {
		t.Errorf("existing keys must be preserved, got %v", cfg)
	}

	// Existing EXPLICIT value → never overwritten (user stays in control).
	if err := os.WriteFile(settings, []byte(`{"effortLevel":"medium"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureClaudeHomeEffortLevel(home)
	if v, _ := readSettings(t, home)["effortLevel"].(string); v != "medium" {
		t.Errorf("explicit effortLevel must survive, got %q", v)
	}

	// Unparseable file → left byte-identical (no clobbering hand edits).
	broken := `{"model": "opus", BROKEN`
	if err := os.WriteFile(settings, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureClaudeHomeEffortLevel(home)
	if b, _ := os.ReadFile(settings); string(b) != broken {
		t.Error("unparseable settings must not be rewritten")
	}
}

// TestCLIEffortLevel: ThinkingLevel → effortLevel mapping. Empty ("Kapalı"),
// "off" and unknown pin "high" — thinking is disabled separately for those
// levels (Request.DisableThinking) and high effort keeps simple-task batching.
func TestCLIEffortLevel(t *testing.T) {
	cases := map[string]string{
		"":        "high",
		"high":    "high",
		"xhigh":   "high",
		"max":     "high",
		"unknown": "high",
		"off":     "high",
		"medium":  "medium",
		"low":     "low",
	}
	for in, want := range cases {
		if got := cliEffortLevel(in); got != want {
			t.Errorf("cliEffortLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWriteCLISettingsCarriesEffort: the per-turn --settings file is now written
// on every MCP-delegated turn and carries the pinned effortLevel even with no
// deny-list and no hooks.
func TestWriteCLISettingsCarriesEffort(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	path, cleanup, err := rt.writeCLISettings(context.Background(), nil, "high")
	if err != nil {
		t.Fatalf("writeCLISettings: %v", err)
	}
	if path == "" {
		t.Fatal("settings file must be written when an effort level is pinned")
	}
	t.Cleanup(cleanup)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	if !strings.Contains(string(b), `"effortLevel": "high"`) {
		t.Errorf("effortLevel missing from per-turn settings: %s", b)
	}
}
