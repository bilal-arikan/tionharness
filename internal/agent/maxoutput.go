package agent

import (
	"os"
	"strconv"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// maxOutputOverride, when > 0, forces the generation cap (max output tokens) for
// every runtime provider call regardless of model family. Set via
// TIONSWARM_MAX_OUTPUT_TOKENS; 0/unset defers to the per-family default. Mirrors the
// TIONSWARM_MAX_TOOL_ITERS / TIONSWARM_MAX_CONTEXT_TOKENS escape hatches.
var maxOutputOverride = envMaxOutputOverride()

func envMaxOutputOverride() int {
	if v, err := strconv.Atoi(os.Getenv("TIONSWARM_MAX_OUTPUT_TOKENS")); err == nil && v > 0 {
		return v
	}
	return 0
}

// withMaxOutput fills req.MaxTokens with a model-aware default when the caller
// left it unset (0), so the output cap tracks the model's real capacity instead
// of the providers' conservative 4096 fallback — the cause of answers being
// truncated mid-stream and resumed by the turn-recovery (A1) loop far more often
// than necessary. An explicit cap (compaction summaries, reflections, titles) is
// a deliberate bound and is never overridden. provider is the agent's provider
// kind, used by providers.MaxOutputFor for family resolution.
//
// Precedence when MaxTokens is unset: the Settings override (Tunables.
// MaxOutputTokens, surfaced in Settings ▸ Context) wins; then the
// TIONSWARM_MAX_OUTPUT_TOKENS env; then the per-model family default. When none
// applies the request is left unset and the provider falls back to its own
// default.
func (r *Runtime) withMaxOutput(provider string, req providers.Request) providers.Request {
	if req.MaxTokens > 0 {
		return req
	}
	if n := r.tun.MaxOutputTokens(); n > 0 {
		req.MaxTokens = n
		return req
	}
	if maxOutputOverride > 0 {
		req.MaxTokens = maxOutputOverride
		return req
	}
	if n := providers.MaxOutputFor(provider, req.Model); n > 0 {
		req.MaxTokens = n
	}
	return req
}
