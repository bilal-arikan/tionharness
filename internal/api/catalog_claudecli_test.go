package api

import (
	"os"
	"path/filepath"
	"testing"
)

// writeClaudeHome creates a claude-home holding a .credentials.json with the
// given subscription type, mirroring the file Claude Code writes on login.
func writeClaudeHome(t *testing.T, subType string) string {
	t.Helper()
	home := t.TempDir()
	body := `{"claudeAiOauth":{"accessToken":"tok","subscriptionType":"` + subType + `"}}`
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestClaudeSubscriptionTier(t *testing.T) {
	t.Run("reads the tier from the workspace claude-home", func(t *testing.T) {
		if got := claudeSubscriptionTier(writeClaudeHome(t, "Max"), ""); got != "max" {
			t.Fatalf("tier = %q, want %q", got, "max")
		}
	})

	// An injected API key is billed per token, so no plan is reported even though
	// a stale OAuth credential is still sitting in the home.
	t.Run("api-key credential reports no plan", func(t *testing.T) {
		if got := claudeSubscriptionTier(writeClaudeHome(t, "max"), "apikey"); got != "" {
			t.Fatalf("tier = %q, want empty", got)
		}
	})

	t.Run("no credential file reports no plan", func(t *testing.T) {
		if got := claudeSubscriptionTier(t.TempDir(), ""); got != "" {
			t.Fatalf("tier = %q, want empty", got)
		}
	})

	// Never guess: a credential that does not name its plan yields nothing rather
	// than defaulting to "max".
	t.Run("missing subscriptionType reports no plan", func(t *testing.T) {
		if got := claudeSubscriptionTier(writeClaudeHome(t, ""), ""); got != "" {
			t.Fatalf("tier = %q, want empty", got)
		}
	})
}

func TestClaudeCLIVersionEmptyPath(t *testing.T) {
	if got := claudeCLIVersion(t.Context(), "  "); got != "" {
		t.Fatalf("version = %q, want empty for a blank path", got)
	}
}
