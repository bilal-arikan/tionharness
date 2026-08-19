package agent

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func TestCLIHomeFallbacksAreAppGlobal(t *testing.T) {
	dataDir := t.TempDir()
	r := &Runtime{dataDir: dataDir}
	if got, want := r.claudeHomeDir(), filepath.Join(dataDir, "claude-home"); got != want {
		t.Fatalf("claude home = %q, want %q", got, want)
	}
	if got, want := r.codexHomeDir(), filepath.Join(dataDir, "codex-home"); got != want {
		t.Fatalf("codex home = %q, want %q", got, want)
	}
}

func TestExplicitCLIConfigDirIsUnchanged(t *testing.T) {
	dedicated := t.TempDir()
	claude := providers.NewClaudeCLI("claude", "", dedicated, "", "")
	codex := providers.NewCodexCLI("codex", "", dedicated)
	if claude.ConfigDir() != dedicated || codex.ConfigDir() != dedicated {
		t.Fatal("explicit configDir changed")
	}
}

func TestMigrateSharedCLIHomes(t *testing.T) {
	for _, provider := range []struct{ home, auth string }{
		{"claude-home", ".credentials.json"},
		{"codex-home", "auth.json"},
	} {
		t.Run(provider.home+"/none", func(t *testing.T) {
			dataDir := t.TempDir()
			if err := MigrateSharedCLIHomes(dataDir, []string{t.TempDir()}, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dataDir, provider.home, provider.auth)); !os.IsNotExist(err) {
				t.Fatalf("unexpected global auth: %v", err)
			}
		})

		t.Run(provider.home+"/one", func(t *testing.T) {
			dataDir, workspace := t.TempDir(), t.TempDir()
			legacy := filepath.Join(workspace, provider.home)
			if err := os.MkdirAll(legacy, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, provider.auth), []byte(`{"token":"workspace"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"kept":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MigrateSharedCLIHomes(dataDir, []string{workspace}, nil); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(dataDir, provider.home, provider.auth)); err != nil || string(got) != `{"token":"workspace"}` {
				t.Fatalf("migrated auth = %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(legacy, provider.auth)); err != nil {
				t.Fatalf("legacy auth removed: %v", err)
			}
		})

		t.Run(provider.home+"/several", func(t *testing.T) {
			dataDir := t.TempDir()
			roots := []string{t.TempDir(), t.TempDir()}
			for _, root := range roots {
				home := filepath.Join(root, provider.home)
				if err := os.MkdirAll(home, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, provider.auth), []byte(`{"token":"different"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			if err := MigrateSharedCLIHomes(dataDir, roots, logger); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dataDir, provider.home, provider.auth)); !os.IsNotExist(err) {
				t.Fatalf("ambiguous login migrated: %v", err)
			}
		})
	}
}
