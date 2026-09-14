package workspace

import (
	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// codexPluginSpecLocked builds the runtime-facing codex plugin spec from the
// current settings. The caller must already hold settings.mu (both call sites —
// loadSettings and UpdateSettings — read several fields under the same lock).
//
// A disabled workspace yields an EMPTY spec rather than the configured lists, so
// every downstream consumer can treat "nothing to render, nothing to install" as
// the single off state instead of re-checking the flag.
func (w *Workspace) codexPluginSpecLocked() agent.CodexPluginSpec {
	s := w.settings.cur
	if !s.CodexPluginsEnabled {
		return agent.CodexPluginSpec{}
	}
	return agent.CodexPluginSpec{
		Marketplaces: append([]db.CodexMarketplace(nil), s.CodexMarketplaces...),
		Plugins:      append([]string(nil), s.CodexPlugins...),
	}
}

// CodexPluginSpec returns this workspace's effective codex plugin configuration
// (empty when the feature is off), for callers outside the settings path.
func (w *Workspace) CodexPluginSpec() agent.CodexPluginSpec {
	w.settings.mu.RLock()
	defer w.settings.mu.RUnlock()
	return w.codexPluginSpecLocked()
}
