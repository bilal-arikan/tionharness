package proc

import (
	"os"
	"sort"
	"strings"
)

// StrippedEnvVar names the marker variable injected into every credential-filtered
// child environment. Its value is the comma-separated, sorted list of variable
// names that were removed (empty when nothing matched). It exists so a stripped
// variable can never break a command SILENTLY: when a subprocess suddenly cannot
// authenticate, `echo $TIONHARNESS_STRIPPED_ENV` names exactly what was withheld,
// without TionHarness having to log a line on every single shell call.
const StrippedEnvVar = "TIONHARNESS_STRIPPED_ENV"

// credentialNameParts are the substrings that mark an environment variable name as
// carrying a secret. Matching is case-insensitive and on the NAME only — a value is
// never inspected. Substring (rather than suffix) matching is deliberate: it also
// catches the middle-of-name forms such as AWS_ACCESS_KEY_ID and
// GOOGLE_CLIENT_SECRET_FILE that a suffix rule would miss.
var credentialNameParts = []string{
	"SECRET",
	"TOKEN",
	"PASSWORD",
	"PASSWD",
	"PASSPHRASE",
	"CREDENTIAL",
	"APIKEY",
	"API_KEY",
	"ACCESS_KEY",
	"PRIVATE_KEY",
	"AUTH_KEY",
	"SESSION_KEY",
	"SIGNING_KEY",
	"_KEY",
}

// credentialNamePrefixes are vendor namespaces whose variables are treated as
// credentials wholesale, even when the name carries none of credentialNameParts
// (e.g. ANTHROPIC_AUTH, OPENAI_ORGANIZATION).
var credentialNamePrefixes = []string{
	"ANTHROPIC_",
	"OPENAI_",
	"OPENROUTER_",
	"AZURE_OPENAI_",
	"GEMINI_",
	"GROQ_",
	"MISTRAL_",
	"DEEPSEEK_",
	"XAI_",
	"HUGGINGFACE_",
	"HF_",
}

// credentialNamesExact are individual variables that carry a secret but match
// neither the substring nor the prefix rules.
var credentialNamesExact = []string{
	"CREDENTIAL_SECRET", // TionHarness's own at-rest encryption key
	"GH_TOKEN",
	"GITHUB_TOKEN",
	"NPM_AUTH",
	"PGPASSFILE",
}

// IsCredentialEnvName reports whether an environment variable NAME looks like it
// carries a credential. It is intentionally a DENYLIST: a shell subprocess needs
// the user's real environment (PATH, HOME, TEMP, proxies, locale, GOPATH,
// JAVA_HOME, toolchain-specific vars…), and an allowlist would silently break
// arbitrary commands the moment a variable nobody enumerated turned out to matter.
// The cost of the denylist is the opposite risk — an unlisted secret shape gets
// through — so the patterns are broad on purpose.
func IsCredentialEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, exact := range credentialNamesExact {
		if upper == exact {
			return true
		}
	}
	for _, prefix := range credentialNamePrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	for _, part := range credentialNameParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

// StripCredentialEnv removes every credential-looking entry from an env slice and
// reports the names it removed (sorted, de-duplicated). Entries with no "=" are
// passed through untouched — malformed input is not this function's business.
func StripCredentialEnv(env []string) (kept []string, stripped []string) {
	return stripCredentialEnv(env, nil)
}

// stripCredentialEnv is StripCredentialEnv with an optional exemption predicate:
// when exempt returns true for a NAME, the variable is passed through even though
// IsCredentialEnvName matched it. Used for subprocesses that are a single known
// binary needing its own vendor credentials (see CredentialSafeEnvExcept).
func stripCredentialEnv(env []string, exempt func(name string) bool) (kept []string, stripped []string) {
	kept = make([]string, 0, len(env))
	seen := map[string]bool{}
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			kept = append(kept, kv)
			continue
		}
		name := kv[:eq]
		// A stale marker from a parent TionHarness process must never be inherited:
		// it would misreport what THIS child had withheld.
		if strings.EqualFold(name, StrippedEnvVar) {
			continue
		}
		if !IsCredentialEnvName(name) {
			kept = append(kept, kv)
			continue
		}
		if exempt != nil && exempt(name) {
			kept = append(kept, kv)
			continue
		}
		if !seen[name] {
			seen[name] = true
			stripped = append(stripped, name)
		}
	}
	sort.Strings(stripped)
	return kept, stripped
}

// CredentialSafeEnv returns base (or the current process environment when base is
// nil) with every credential-looking variable removed and the StrippedEnvVar marker
// appended listing what was removed.
//
// This is the environment for subprocesses the AGENT drives — above all the shell
// tools. Without it every `env`, `printenv` or `docker run --env-file /dev/environ`
// the model runs would dump ANTHROPIC_API_KEY, CREDENTIAL_SECRET and every personal
// token straight into the transcript, the UI and the next LLM request.
func CredentialSafeEnv(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	kept, stripped := StripCredentialEnv(base)
	return append(kept, StrippedEnvVar+"="+strings.Join(stripped, ","))
}

// CredentialSafeEnvExcept is CredentialSafeEnv with an exemption predicate: a
// credential-looking NAME for which exempt returns true is passed through to the
// child untouched.
//
// It exists for a threat model that differs from the shell tool's. The shell runs
// ARBITRARY model-authored commands, so nothing secret may reach it. A CLI provider
// subprocess is a SINGLE known binary (claude / codex) that authenticates from its
// own vendor namespace — stripping that namespace does not harden anything, it just
// logs the user out. So the caller exempts exactly the vendor's own variables and
// everything else (the user's GITHUB_TOKEN, AWS keys, TionHarness's own
// CREDENTIAL_SECRET, other vendors' keys) is still withheld.
//
// The StrippedEnvVar marker still lists what was removed, so an exemption that turns
// out to be too narrow is discoverable from inside the child rather than silent.
func CredentialSafeEnvExcept(base []string, exempt func(name string) bool) []string {
	if base == nil {
		base = os.Environ()
	}
	kept, stripped := stripCredentialEnv(base, exempt)
	return append(kept, StrippedEnvVar+"="+strings.Join(stripped, ","))
}
