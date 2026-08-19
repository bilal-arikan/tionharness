package agent

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// writeAuth writes an auth file that the migration accepts as a usable login:
// a real oauth credential shape for claude (a token-less stub is deliberately
// rejected), any non-empty content for codex.
func writeAuth(t *testing.T, home, authName, token string, expiresAt time.Time) string {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"token":"` + token + `"}`)
	if authName == ".credentials.json" {
		body = []byte(fmt.Sprintf(
			`{"claudeAiOauth":{"accessToken":%q,"refreshToken":"refresh","expiresAt":%d,"refreshTokenExpiresAt":%d}}`,
			token, expiresAt.UnixMilli(), expiresAt.Add(24*time.Hour).UnixMilli()))
	}
	path := filepath.Join(home, authName)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	// The codex ranking is by mtime, so make it deterministic instead of relying
	// on how fast the two writes above land.
	if err := os.Chtimes(path, expiresAt, expiresAt); err != nil {
		t.Fatal(err)
	}
	return string(body)
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
			want := writeAuth(t, legacy, provider.auth, "workspace", time.Now().Add(time.Hour))
			if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"kept":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MigrateSharedCLIHomes(dataDir, []string{workspace}, nil); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(dataDir, provider.home, provider.auth)); err != nil || string(got) != want {
				t.Fatalf("migrated auth = %q, %v", got, err)
			}
			// An empty destination is seeded with the whole home, settings included.
			if _, err := os.Stat(filepath.Join(dataDir, provider.home, "settings.json")); err != nil {
				t.Fatalf("settings not seeded into an empty home: %v", err)
			}
			if _, err := os.Stat(filepath.Join(legacy, provider.auth)); err != nil {
				t.Fatalf("legacy auth removed: %v", err)
			}
		})

		// Several workspace logins are the NORM (one home per workspace, same
		// account): the migration must pick the freshest rather than give up and
		// leave the shared home unauthenticated.
		t.Run(provider.home+"/several", func(t *testing.T) {
			dataDir := t.TempDir()
			stale, fresh := t.TempDir(), t.TempDir()
			writeAuth(t, filepath.Join(stale, provider.home), provider.auth, "stale", time.Now().Add(-48*time.Hour))
			want := writeAuth(t, filepath.Join(fresh, provider.home), provider.auth, "fresh", time.Now().Add(time.Hour))

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			if err := MigrateSharedCLIHomes(dataDir, []string{stale, fresh}, logger); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(dataDir, provider.home, provider.auth)); err != nil || string(got) != want {
				t.Fatalf("migrated auth = %q, %v (want the freshest login)", got, err)
			}
		})

		// A home that already exists but was never logged in (settings/caches from
		// an older build) must still receive the credential — and ONLY the
		// credential: the rest of a workspace home is that workspace's own state.
		t.Run(provider.home+"/existing-home-without-login", func(t *testing.T) {
			dataDir, workspace := t.TempDir(), t.TempDir()
			shared := filepath.Join(dataDir, provider.home)
			if err := os.MkdirAll(filepath.Join(shared, "projects"), 0o755); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(workspace, provider.home)
			want := writeAuth(t, legacy, provider.auth, "workspace", time.Now().Add(time.Hour))
			if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"kept":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MigrateSharedCLIHomes(dataDir, []string{workspace}, nil); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(shared, provider.auth)); err != nil || string(got) != want {
				t.Fatalf("migrated auth = %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(shared, "settings.json")); !os.IsNotExist(err) {
				t.Fatalf("non-empty home should receive the credential only: %v", err)
			}
		})

		// An existing, usable login is never overwritten.
		t.Run(provider.home+"/already-logged-in", func(t *testing.T) {
			dataDir, workspace := t.TempDir(), t.TempDir()
			want := writeAuth(t, filepath.Join(dataDir, provider.home), provider.auth, "shared", time.Now().Add(time.Hour))
			writeAuth(t, filepath.Join(workspace, provider.home), provider.auth, "workspace", time.Now().Add(2*time.Hour))
			if err := MigrateSharedCLIHomes(dataDir, []string{workspace}, nil); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(filepath.Join(dataDir, provider.home, provider.auth)); string(got) != want {
				t.Fatalf("existing shared login was replaced: %q", got)
			}
		})
	}

	// A dead claude credential (refresh token recorded as expired) is not a
	// migration source: copying it just moves the login prompt one turn later.
	t.Run("claude-home/unusable-source", func(t *testing.T) {
		dataDir, workspace := t.TempDir(), t.TempDir()
		writeAuth(t, filepath.Join(workspace, "claude-home"), ".credentials.json", "dead", time.Now().Add(-72*time.Hour))
		if err := MigrateSharedCLIHomes(dataDir, []string{workspace}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, "claude-home", ".credentials.json")); !os.IsNotExist(err) {
			t.Fatalf("dead credential migrated: %v", err)
		}
	})
}
