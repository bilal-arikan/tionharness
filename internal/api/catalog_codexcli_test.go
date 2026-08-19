package api

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCodexSubscriptionTier pins the credential check that decides whether the
// Providers screen may claim a codex-home is logged in. The regression it
// guards: an auth.json that exists and parses but carries NO credential
// material was reported as a valid ChatGPT login, so the UI showed
// "✓ giriş yapılmış" for a home where every turn failed with 401.
func TestCodexSubscriptionTier(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"chatgpt tokens", `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"at","refresh_token":"rt"}}`, "chatgpt"},
		{"refresh token only", `{"tokens":{"refresh_token":"rt"}}`, "chatgpt"},
		{"api key", `{"OPENAI_API_KEY":"sk-live"}`, "apikey"},
		{"empty object", `{}`, ""},
		{"null credentials", `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"","refresh_token":""}}`, ""},
		{"blank api key", `{"OPENAI_API_KEY":"   "}`, ""},
		{"malformed", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, codexAuthFile), []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := codexSubscriptionTier(home); got != tc.want {
				t.Errorf("codexSubscriptionTier() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCodexSubscriptionTierNoFile: a codex-home that was never logged into has
// no auth.json at all, and an empty home dir must never read as logged in.
func TestCodexSubscriptionTierNoFile(t *testing.T) {
	if got := codexSubscriptionTier(t.TempDir()); got != "" {
		t.Errorf("missing auth.json reported %q, want empty", got)
	}
	if got := codexSubscriptionTier(""); got != "" {
		t.Errorf("empty home reported %q, want empty", got)
	}
}
