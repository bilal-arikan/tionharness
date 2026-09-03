package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegistryConsistency is the drift guard: every registered key must have a
// non-empty embedded default that itself contains every required placeholder,
// keys must be unique, and every defaults/*.md file must be registered (an
// orphan file means someone added a prompt without a Spec, or vice versa).
func TestRegistryConsistency(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Specs() {
		if s.Key == "" || s.Label == "" {
			t.Errorf("spec %+v: key and label are required", s)
		}
		if seen[s.Key] {
			t.Errorf("duplicate key %q", s.Key)
		}
		seen[s.Key] = true

		def := Default(s.Key)
		if strings.TrimSpace(def) == "" {
			t.Errorf("key %q: embedded default is empty", s.Key)
		}
		if err := Validate(s.Key, def); err != nil {
			t.Errorf("key %q: embedded default fails its own validation: %v", s.Key, err)
		}
	}

	entries, err := defaultsFS.ReadDir("defaults")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		key := strings.TrimSuffix(e.Name(), ".md")
		if !seen[key] {
			t.Errorf("defaults/%s exists but key %q is not registered in specs", e.Name(), key)
		}
	}
	if len(entries) != len(specs) {
		t.Errorf("defaults/ has %d files but registry has %d specs", len(entries), len(specs))
	}
}

func TestSystemAgentOwnership(t *testing.T) {
	want := map[string]string{
		"title":              "titler",
		"summary":            "overview-summarizer",
		"compact":            "compaction",
		"lesson":             "lesson-extractor",
		"insight-analyzer":   "insight",
		"insight-applier":    "insight-applier",
		"recipe-optimizer":   "recipe-optimizer",
		"stall-judge":        "stall-judge",
		"subagent-explore":   "subagent-explore",
		"subagent-planner":   "subagent-planner",
		"subagent-coder":     "subagent-coder",
		"subagent-reviewer":  "subagent-reviewer",
		"subagent-validator": "subagent-validator",
		"subagent-config":    "subagent-config",
	}
	for _, s := range Specs() {
		if got, owned := want[s.Key]; owned {
			if s.OwnedBySystemKey != got {
				t.Errorf("prompt %q ownedBySystemKey = %q, want %q", s.Key, s.OwnedBySystemKey, got)
			}
			delete(want, s.Key)
		} else if s.OwnedBySystemKey != "" {
			t.Errorf("prompt %q unexpectedly owned by system agent %q", s.Key, s.OwnedBySystemKey)
		}
	}
	for key := range want {
		t.Errorf("owned prompt %q is not registered", key)
	}
}

func TestRender(t *testing.T) {
	got := Render("a {{x}} b {{y}} c {{x}}", map[string]string{"x": "1", "y": "2"})
	if got != "a 1 b 2 c 1" {
		t.Fatalf("Render = %q", got)
	}
	// Unknown placeholders stay literal (visible, not silently dropped).
	if got := Render("keep {{unknown}}", map[string]string{"x": "1"}); got != "keep {{unknown}}" {
		t.Fatalf("Render unknown = %q", got)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("compact", "has {{summary}} and {{messages}}"); err != nil {
		t.Fatalf("valid compact rejected: %v", err)
	}
	if err := Validate("compact", "missing both"); err == nil {
		t.Fatal("compact without placeholders accepted")
	}
	if err := Validate("summary", "  "); err == nil {
		t.Fatal("blank override accepted")
	}
	if err := Validate("no-such-key", "text"); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestResolveFile_FallbackAndOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.md")

	// Missing file → default.
	if got := ResolveFile(path, "summary"); got != Default("summary") {
		t.Fatalf("missing file: got %q", got)
	}
	// Valid override wins.
	if err := os.WriteFile(path, []byte("CUSTOM"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveFile(path, "summary"); got != "CUSTOM" {
		t.Fatalf("override: got %q", got)
	}
	// Blank file → default.
	if err := os.WriteFile(path, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveFile(path, "summary"); got != Default("summary") {
		t.Fatalf("blank file: got %q", got)
	}

	// A compact override that drops a placeholder falls back to the default.
	cpath := filepath.Join(dir, "compact.md")
	if err := os.WriteFile(cpath, []byte("only {{summary}} here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveFile(cpath, "compact"); got != Default("compact") {
		t.Fatalf("invalid compact override did not fall back")
	}
}

// TestResolveFile_LegacyCompact locks the migration path: a pre-registry
// compact.md with two %s slots is converted to named placeholders on read.
func TestResolveFile_LegacyCompact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compact.md")
	legacy := "Merge EXISTING:\n%s\nNEW:\n%s\nDone."
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveFile(path, "compact")
	if !strings.Contains(got, "{{summary}}") || !strings.Contains(got, "{{messages}}") {
		t.Fatalf("legacy compact not converted: %q", got)
	}
	if strings.Contains(got, "%s") {
		t.Fatalf("legacy %%s slot survived conversion: %q", got)
	}
}
