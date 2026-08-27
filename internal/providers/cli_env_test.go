package providers

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// envMap turns a KEY=VALUE slice into a lookup, last write winning (which is how
// the OS resolves duplicates in exec's env).
func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

// TestCLIEnvExemptTable pins exactly which credential-looking names each CLI
// provider is allowed to inherit. A change here is a deliberate security decision,
// not an incidental one.
func TestCLIEnvExemptTable(t *testing.T) {
	cases := []struct {
		name   string
		claude bool
		codex  bool
	}{
		// Vendor auth/endpoint namespaces — exempt for their OWN CLI only.
		{"ANTHROPIC_API_KEY", true, false},
		{"ANTHROPIC_AUTH_TOKEN", true, false},
		{"ANTHROPIC_BASE_URL", true, false},
		{"anthropic_api_key", true, false}, // matching is case-insensitive
		{"OPENAI_API_KEY", false, true},
		{"OPENAI_BASE_URL", false, true},

		// Never exempt anywhere: unrelated to either CLI's auth.
		{"CREDENTIAL_SECRET", false, false},
		{"GITHUB_TOKEN", false, false},
		{"GH_TOKEN", false, false},
		{"AWS_SECRET_ACCESS_KEY", false, false},
		{"NPM_AUTH", false, false},
		{"GEMINI_API_KEY", false, false},
		{"MY_PRIVATE_KEY", false, false},

		// Not credentials at all — they never reach the predicate in practice, but
		// the predicate must not claim them either.
		{"PATH", false, false},
		{"CODEX_HOME", false, false},
	}
	for _, tc := range cases {
		if got := claudeCLIEnvExempt(tc.name); got != tc.claude {
			t.Errorf("claudeCLIEnvExempt(%q) = %v, want %v", tc.name, got, tc.claude)
		}
		if got := codexCLIEnvExempt(tc.name); got != tc.codex {
			t.Errorf("codexCLIEnvExempt(%q) = %v, want %v", tc.name, got, tc.codex)
		}
	}
}

// TestCLIBaseEnvFiltersCredentials exercises the real cliBaseEnv/codexBaseEnv over a
// process environment carrying both the CLI's own credential and foreign ones.
func TestCLIBaseEnvFiltersCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("OPENAI_API_KEY", "sk-oai-secret")
	t.Setenv("CREDENTIAL_SECRET", "at-rest-key")
	t.Setenv("GITHUB_TOKEN", "ghp-secret")
	t.Setenv("ANTHROPIC_MODEL", "haiku") // nesting leak: dropped before the filter
	t.Setenv("TIONHARNESS_TEST_PLAIN", "keep-me")

	t.Run("claude", func(t *testing.T) {
		m := envMap(cliBaseEnv("ENABLE_TOOL_SEARCH=auto"))
		if m["ANTHROPIC_API_KEY"] != "sk-ant-secret" {
			t.Errorf("claude CLI lost its own credential: %q", m["ANTHROPIC_API_KEY"])
		}
		for _, gone := range []string{"OPENAI_API_KEY", "CREDENTIAL_SECRET", "GITHUB_TOKEN", "ANTHROPIC_MODEL"} {
			if _, ok := m[gone]; ok {
				t.Errorf("%s reached the claude CLI subprocess", gone)
			}
		}
		if m["TIONHARNESS_TEST_PLAIN"] != "keep-me" {
			t.Error("non-credential variable was dropped")
		}
		if m["ENABLE_TOOL_SEARCH"] != "auto" {
			t.Error("extra variable was dropped")
		}
		// Not silent: the child can name what was withheld.
		if !strings.Contains(m[proc.StrippedEnvVar], "GITHUB_TOKEN") {
			t.Errorf("%s does not report the stripped names: %q", proc.StrippedEnvVar, m[proc.StrippedEnvVar])
		}
	})

	t.Run("codex", func(t *testing.T) {
		m := envMap(codexBaseEnv())
		if m["OPENAI_API_KEY"] != "sk-oai-secret" {
			t.Errorf("codex CLI lost its own credential: %q", m["OPENAI_API_KEY"])
		}
		for _, gone := range []string{"ANTHROPIC_API_KEY", "CREDENTIAL_SECRET", "GITHUB_TOKEN"} {
			if _, ok := m[gone]; ok {
				t.Errorf("%s reached the codex CLI subprocess", gone)
			}
		}
		if m["TIONHARNESS_TEST_PLAIN"] != "keep-me" {
			t.Error("non-credential variable was dropped")
		}
		if !strings.Contains(m[proc.StrippedEnvVar], "ANTHROPIC_API_KEY") {
			t.Errorf("%s does not report the stripped names: %q", proc.StrippedEnvVar, m[proc.StrippedEnvVar])
		}
	})
}
