package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceConfig_SeedAndReadPrompt(t *testing.T) {
	wsDir := t.TempDir()
	workDir := filepath.Join(wsDir, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{workDir: workDir}

	// Before seeding, a missing file falls back to the compiled-in default.
	if got := r.readPrompt("summary"); got != summarySystemPrompt {
		t.Fatalf("pre-seed readPrompt = %q, want default", got)
	}

	if err := SeedWorkspaceConfig(wsDir, "be terse"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Seeding writes each prompt default + the instructions file.
	for _, key := range PromptKeys {
		data, err := os.ReadFile(PromptFilePath(wsDir, key))
		if err != nil {
			t.Fatalf("seeded prompt %s missing: %v", key, err)
		}
		if string(data) != promptDefaults[key] {
			t.Fatalf("seeded prompt %s != default", key)
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

	// A blanked file falls back to the default again.
	if err := os.WriteFile(PromptFilePath(wsDir, "summary"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.readPrompt("summary"); got != summarySystemPrompt {
		t.Fatalf("blank readPrompt = %q, want default", got)
	}

	// Re-seeding never clobbers existing files (user edits survive).
	if err := os.WriteFile(PromptFilePath(wsDir, "title"), []byte("KEEP ME"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedWorkspaceConfig(wsDir, "ignored"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(PromptFilePath(wsDir, "title")); string(data) != "KEEP ME" {
		t.Fatalf("re-seed clobbered edited file: %q", string(data))
	}
}

func TestRuntimeConfigDir_EmptyWorkDir(t *testing.T) {
	r := &Runtime{}
	if dir := r.configDir(); dir != "" {
		t.Fatalf("configDir with empty workDir = %q, want empty", dir)
	}
	if got := r.readPrompt("title"); got != titleSystemPrompt {
		t.Fatalf("readPrompt with no workDir = %q, want default", got)
	}
}
