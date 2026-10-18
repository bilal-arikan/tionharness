package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/settings"
)

func TestCodexHomesCoversGlobalAndInstanceDirs(t *testing.T) {
	data := t.TempDir()
	own := filepath.Join(t.TempDir(), "work-codex")
	got := codexHomes(data, []settings.ProviderInstance{
		{KindID: "codex-cli"}, // uses the app-global home
		{KindID: "codex-cli", Config: map[string]string{"configDir": own + "/"}},
		{KindID: "codex-cli", Config: map[string]string{"configDir": own}},
		{KindID: "claude-cli", Config: map[string]string{"configDir": filepath.Join(data, "claude-home")}},
	})
	want := []string{filepath.Join(data, "codex-home"), own}
	if !slices.Equal(got, want) {
		t.Fatalf("codexHomes = %v, want %v", got, want)
	}
}
