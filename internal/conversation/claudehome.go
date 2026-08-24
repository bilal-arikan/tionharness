package conversation

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// claudeHomeKey carries THIS workspace's claude-cli config home on the fold
// context so the compaction/handoff core can pin it on the shared claude-cli
// provider right before its direct provider.Complete call.
type claudeHomeKey struct{}

// WithClaudeHome returns a context carrying the workspace claude-cli config home
// (<workspace>/claude-home). The fold core (summarizeRendered, BuildHandoff)
// reads it to pin the shared claude-cli provider before its direct
// provider.Complete — without pinning the CLI falls back to the GLOBAL
// claude-home and fails auth (authentication_failed) even when the workspace is
// logged in. Set it alongside WithCompactPrompt at every fold entry point; a
// blank dir is a no-op (keeps the provider's default config dir).
func WithClaudeHome(ctx context.Context, dir string) context.Context {
	if dir == "" {
		return ctx
	}
	return context.WithValue(ctx, claudeHomeKey{}, dir)
}

func claudeHomeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(claudeHomeKey{}).(string); ok {
		return v
	}
	return ""
}

// pinClaudeHome pins the workspace claude-home carried on ctx onto a claude-cli
// provider, mirroring agent.Runtime.PinClaudeHome but self-contained in the fold
// core so no fold call site can silently fall back to the global home. No-op for
// non-claude-cli providers or when no home was set on ctx (the explicit
// PinClaudeHome at the acquisition site still covers that case).
func pinClaudeHome(ctx context.Context, provider providers.Provider) {
	if cli, ok := provider.(*providers.ClaudeCLI); ok {
		if home := claudeHomeFrom(ctx); home != "" {
			cli.SetConfigDir(home)
		}
	}
}
