package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/prompts"
)

func TestWorkspaceConfig_SeedAndReadPrompt(t *testing.T) {
	wsDir := t.TempDir()
	workDir := filepath.Join(wsDir, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{workDir: workDir}

	// Before seeding, a missing file falls back to the registry default.
	if got := r.readPrompt("summary"); got != prompts.Default("summary") {
		t.Fatalf("pre-seed readPrompt = %q, want default", got)
	}

	if err := SeedWorkspaceConfig(wsDir, "be terse"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Seeding writes the instructions file + README but NO prompt files —
	// resolution falls back to the embedded defaults, so shipped default
	// improvements reach every workspace that has not edited the prompt.
	for _, key := range PromptKeys {
		if _, err := os.Stat(PromptFilePath(wsDir, key)); err == nil {
			t.Fatalf("seed wrote prompt file for %s — defaults must stay embedded-only", key)
		}
		if got := WorkspacePrompt(wsDir, key); got != prompts.Default(key) {
			t.Fatalf("post-seed %s != default", key)
		}
	}
	if data, _ := os.ReadFile(InstructionsFilePath(wsDir)); string(data) != "be terse" {
		t.Fatalf("instructions not seeded: %q", string(data))
	}

	// An edited file wins over the default.
	if err := os.WriteFile(PromptFilePath(wsDir, "summary"), []byte("CUSTOM PROMPT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.readPrompt("summary"); got != "CUSTOM PROMPT" {
		t.Fatalf("edited readPrompt = %q, want CUSTOM PROMPT", got)
	}
	if got := WorkspacePrompt(wsDir, "summary"); got != "CUSTOM PROMPT" {
		t.Fatalf("edited WorkspacePrompt = %q, want CUSTOM PROMPT", got)
	}

	// A blanked file falls back to the default again.
	if err := os.WriteFile(PromptFilePath(wsDir, "summary"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.readPrompt("summary"); got != prompts.Default("summary") {
		t.Fatalf("blank readPrompt = %q, want default", got)
	}

	// Re-seeding never clobbers edited files (user edits survive)…
	if err := os.WriteFile(PromptFilePath(wsDir, "title"), []byte("KEEP ME"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedWorkspaceConfig(wsDir, "ignored"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(PromptFilePath(wsDir, "title")); string(data) != "KEEP ME" {
		t.Fatalf("re-seed clobbered edited file: %q", string(data))
	}

	// …but an old-build seed artifact (file identical to the current default)
	// is cleaned up, so the workspace tracks future default improvements.
	if err := os.WriteFile(PromptFilePath(wsDir, "lesson"), []byte(prompts.Default("lesson")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedWorkspaceConfig(wsDir, "ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PromptFilePath(wsDir, "lesson")); err == nil {
		t.Fatal("default-identical prompt file survived seeding — should be cleaned up")
	}

	// A pre-registry compact seed (old default with two %s slots) is ALSO an
	// artifact, not a user edit — cleanup must recognize it via legacy
	// normalization and remove it.
	legacy := prompts.Default("compact")
	legacy = strings.Replace(legacy, "{{summary}}", "%s", 1)
	legacy = strings.Replace(legacy, "{{messages}}", "%s", 1)
	if err := os.WriteFile(PromptFilePath(wsDir, "compact"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedWorkspaceConfig(wsDir, "ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PromptFilePath(wsDir, "compact")); err == nil {
		t.Fatal("legacy percent-s form compact seed survived seeding — should be cleaned up")
	}
}

func TestRuntimeConfigDir_EmptyWorkDir(t *testing.T) {
	r := &Runtime{}
	if dir := r.configDir(); dir != "" {
		t.Fatalf("configDir with empty workDir = %q, want empty", dir)
	}
	if got := r.readPrompt("title"); got != prompts.Default("title") {
		t.Fatalf("readPrompt with no workDir = %q, want default", got)
	}
}
