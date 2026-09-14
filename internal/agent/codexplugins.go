package agent

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// CodexPluginSpec is one workspace's effective codex-cli plugin configuration:
// the marketplaces to declare and the plugin selectors to enable. The zero value
// is the off state — nothing rendered, nothing installed.
//
// It lives in the agent package rather than in workspace because both the
// workspace settings path (which produces it) and the provider wiring (which
// consumes it) already depend on agent, while providers must NOT depend on db.
type CodexPluginSpec struct {
	Marketplaces []db.CodexMarketplace
	Plugins      []string
}

// Empty reports whether the spec asks for nothing. Callers use it to skip the
// whole plugin path — including the install step, which would otherwise shell
// out to codex once per conversation home for no reason.
func (s CodexPluginSpec) Empty() bool {
	return len(s.Marketplaces) == 0 && len(s.Plugins) == 0
}

// providerMarketplaces converts the db-shaped marketplaces into the provider
// package's own type. The duplication is deliberate: internal/providers renders
// codex's config.toml and must stay free of a db import (see the dependency
// direction enforced by scripts/depcheck.sh).
func (s CodexPluginSpec) providerMarketplaces() []providers.CodexMarketplace {
	if len(s.Marketplaces) == 0 {
		return nil
	}
	out := make([]providers.CodexMarketplace, 0, len(s.Marketplaces))
	for _, m := range s.Marketplaces {
		out = append(out, providers.CodexMarketplace{
			Name:       m.Name,
			Source:     m.Source,
			SourceType: m.SourceType,
			Ref:        m.Ref,
		})
	}
	return out
}

// applyCodexPlugins installs this workspace's plugin configuration onto a codex
// provider. Non-codex providers are ignored, so callers can hand it any Provider.
//
// Called once per turn alongside PinCodexHome: the spec is workspace state that
// can change between turns (the user toggles the feature or edits the lists),
// and the provider instance is shared, so it must be refreshed rather than baked
// in at construction.
func (r *Runtime) applyCodexPlugins(provider providers.Provider) {
	cx, ok := provider.(*providers.CodexCLI)
	if !ok {
		return
	}
	spec := r.CodexPluginSpec()
	cx.SetCodexPlugins(spec.providerMarketplaces(), spec.Plugins)
}
