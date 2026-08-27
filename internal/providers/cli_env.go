package providers

import "strings"

// The CLI providers spawn a single, known vendor binary — not the arbitrary
// model-authored command line the shell tool runs. So their child environment is
// credential-filtered like the shell's (proc.CredentialSafeEnv), but with a narrow
// exemption for the ONE vendor namespace that binary authenticates from. Stripping
// that namespace would not harden anything: it would just log the provider out.
//
// Everything outside the exemption is still withheld — the user's GITHUB_TOKEN /
// GH_TOKEN, AWS_*/*_ACCESS_KEY, TionHarness's own CREDENTIAL_SECRET, and every OTHER
// vendor's keys, none of which the CLI needs. What was withheld stays discoverable
// from inside the child via proc.StrippedEnvVar.

// claudeCLIEnvExempt keeps the claude CLI's own credential/endpoint namespace.
//
// Evidence for the exemption: the CLI's documented auth precedence is
// ANTHROPIC_API_KEY > CLAUDE_CODE_OAUTH_TOKEN > the config dir's .credentials.json
// (_Docs/05-ARSIV.md), and TionHarness itself injects ANTHROPIC_API_KEY for
// authKind="apikey" (claudecli.go). A user who authenticates by exporting
// ANTHROPIC_API_KEY instead of configuring authKind relies on inheritance, so
// stripping it breaks a supported setup. The rest of the prefix (ANTHROPIC_AUTH_TOKEN,
// ANTHROPIC_BASE_URL and the Bedrock/Vertex endpoint vars) is the same story: it is
// how a gateway/proxy deployment reaches its endpoint at all.
//
// The nesting/model-override members of the prefix are NOT re-admitted here:
// cliBaseEnv drops ANTHROPIC_MODEL, ANTHROPIC_SMALL_FAST_MODEL and ANTHROPIC_DEFAULT_*
// (and the whole CLAUDE_CODE_* prefix, including an inherited CLAUDE_CODE_OAUTH_TOKEN)
// before the credential filter runs, and this predicate only decides what survives it.
func claudeCLIEnvExempt(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), "ANTHROPIC_")
}

// codexCLIEnvExempt keeps the codex CLI's own credential/endpoint namespace.
//
// Evidence for the exemption: OPENAI_API_KEY is a documented codex login channel
// (_Docs/69-CODEX-CLI-SAGLAYICI.md), and unlike claude-cli, TionHarness has NO
// backend channel that injects a codex credential (same doc, and _Docs/70-…:492) —
// an inherited OPENAI_API_KEY or a prior `codex login` is the ONLY way the child
// authenticates. Stripping the prefix would therefore break every API-key user.
func codexCLIEnvExempt(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), "OPENAI_")
}
