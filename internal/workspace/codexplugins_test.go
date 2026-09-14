package workspace

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestDefaultWSSettingsEnablesCodexPlugins(t *testing.T) {
	// Default ON: the feature is the master switch, not the content. A fresh
	// workspace still has no marketplace, so nothing is rendered or installed.
	s := defaultWSSettings()
	if !s.CodexPluginsEnabled {
		t.Fatal("codex plugin support must default to enabled")
	}
	if len(s.CodexMarketplaces) != 0 || len(s.CodexPlugins) != 0 {
		t.Fatal("a fresh workspace must ship no marketplace or plugin of its own")
	}
}

func TestCodexPluginSpecEmptyWhenDisabled(t *testing.T) {
	w := &Workspace{}
	w.settings.cur = WSSettings{
		CodexPluginsEnabled: false,
		CodexMarketplaces:   []db.CodexMarketplace{{Name: "mp", Source: "/mp", SourceType: "local"}},
		CodexPlugins:        []string{"visualize@mp"},
	}
	// Disabling must hide the configured lists rather than merely skipping the
	// install, so every consumer sees one single off state.
	if spec := w.CodexPluginSpec(); !spec.Empty() {
		t.Fatalf("disabled workspace must yield an empty spec, got %+v", spec)
	}
}

func TestCodexPluginSpecCarriesConfigWhenEnabled(t *testing.T) {
	w := &Workspace{}
	w.settings.cur = WSSettings{
		CodexPluginsEnabled: true,
		CodexMarketplaces:   []db.CodexMarketplace{{Name: "mp", Source: "/mp", SourceType: "local"}},
		CodexPlugins:        []string{"visualize@mp"},
	}
	spec := w.CodexPluginSpec()
	if spec.Empty() || len(spec.Marketplaces) != 1 || len(spec.Plugins) != 1 {
		t.Fatalf("enabled workspace must carry its configuration, got %+v", spec)
	}
	// The spec must be a copy: a caller mutating it cannot reach into settings.
	spec.Marketplaces[0].Name = "mutated"
	if w.settings.cur.CodexMarketplaces[0].Name != "mp" {
		t.Fatal("spec must not alias the stored settings")
	}
}
