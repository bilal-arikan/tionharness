package providers

import "strings"

// Failure classification for the codex-cli transport.
//
// Codex reports a failed turn as a stream event (turn.failed / error) whose
// message is the raw upstream text. Distinguishing the classes matters for two
// reasons:
//
//   - Retry safety. A "clean crash" (the process died before producing anything)
//     is safe to re-run; an auth or quota rejection hits the same wall in
//     milliseconds and only burns the second attempt.
//   - Early kill. On a 401 Codex retries INTERNALLY — 5× over websocket then 5×
//     over https, roughly 35 s of wasted stream — before it gives up. Detecting
//     the signature on the FIRST error event lets the provider kill the
//     subprocess instead of waiting that out.

// codexFailureClass names why a codex turn failed.
type codexFailureClass int

const (
	// codexFailureNone means the text carries no recognised terminal signature.
	codexFailureNone codexFailureClass = iota
	// codexFailureAuth: this CODEX_HOME has no valid login (never ran
	// `codex login`, or the credential expired / was revoked).
	codexFailureAuth
	// codexFailureQuota: rate limit, usage limit, or the selected model being at
	// capacity. Retrying immediately hits the same wall.
	codexFailureQuota
	// codexFailureModel: the requested model is not usable with this account
	// (e.g. a slug that needs API billing on a ChatGPT-account login).
	codexFailureModel
)

// retryable reports whether re-running the turn could plausibly succeed. Every
// classified failure is terminal — the class exists precisely to stop a futile
// retry — so only codexFailureNone leaves the decision to the caller.
func (c codexFailureClass) retryable() bool { return c == codexFailureNone }

// classifyCodexError maps a codex error message onto a failure class. Matching
// is case-insensitive substring matching against the signatures observed live;
// an unrecognised message yields codexFailureNone so the caller falls back to
// its own (stream-shape based) retry decision rather than guessing.
func classifyCodexError(msg string) codexFailureClass {
	s := strings.ToLower(msg)
	if s == "" {
		return codexFailureNone
	}
	switch {
	case isCodexAuthText(s):
		return codexFailureAuth
	// Checked before the quota class: the ChatGPT-account rejection names a
	// model and must not be reported as a usage limit the user cannot act on.
	case strings.Contains(s, "not supported when using codex with a chatgpt account"),
		strings.Contains(s, "model is not supported"):
		return codexFailureModel
	case strings.Contains(s, "rate limit"),
		strings.Contains(s, "rate_limit"),
		strings.Contains(s, "usage limit"),
		strings.Contains(s, "usage_limit"),
		strings.Contains(s, "quota"),
		strings.Contains(s, "at capacity"):
		return codexFailureQuota
	}
	return codexFailureNone
}

// isCodexAuthText reports whether s (already lower-cased) signals an
// authentication failure. The live 401 body is
// "unexpected status 401 Unauthorized: Missing bearer or basic authentication
// in header, ..." — matched on both the status and the bearer wording so a
// reworded body still classifies.
func isCodexAuthText(s string) bool {
	return strings.Contains(s, "401 unauthorized") ||
		strings.Contains(s, "missing bearer") ||
		strings.Contains(s, "not logged in") ||
		strings.Contains(s, "please run `codex login`") ||
		strings.Contains(s, "please run codex login") ||
		strings.Contains(s, "invalid api key") ||
		// A revoked/expired OAuth credential: "Your access token could not be
		// refreshed because your refresh token was revoked. Please log out and
		// sign in again." Both halves are matched so a reworded body (expired
		// instead of revoked, access token instead of refresh token) still lands
		// in the auth class and gets the actionable CODEX_HOME message.
		strings.Contains(s, "refresh token") ||
		strings.Contains(s, "could not be refreshed") ||
		strings.Contains(s, "unauthorized")
}

// describeCodexFailure renders an actionable, user-facing explanation for a
// classified failure. configDir names the CODEX_HOME the login must be created
// in, since that is the single most common thing to get wrong: a workspace's
// codex-home is not the ambient one the user logged into by hand.
func describeCodexFailure(class codexFailureClass, msg, configDir string) string {
	home := configDir
	if home == "" {
		home = "the CLI's default config dir (~/.codex)"
	}
	switch class {
	case codexFailureAuth:
		return "codex CLI authentication failed (" + msg + "): this workspace's codex-home is not logged in — " +
			"run `codex login` with CODEX_HOME=" + home + ", or switch this agent to an API-key provider"
	case codexFailureQuota:
		return "codex CLI usage/rate limit reached: " + msg
	case codexFailureModel:
		return "codex CLI model unavailable: " + msg + " — pick another model for this agent"
	}
	return msg
}
