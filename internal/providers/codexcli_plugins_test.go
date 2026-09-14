package providers

import (
	"strings"
	"testing"
)

func TestCodexConfigRendersMarketplacesAndPlugins(t *testing.T) {
	got := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{
			{Name: "local-openai", Source: `C:\Users\x\.codex\mp`, SourceType: "local"},
		},
		Plugins: []string{"visualize@local-openai"},
	})
	for _, want := range []string{
		"[marketplaces.local-openai]\n",
		`source_type = "local"` + "\n",
		// A Windows path MUST be a literal string: inside a basic string TOML reads
		// \U as a unicode escape and rejects the whole config, which under
		// --strict-config kills every turn.
		`source = 'C:\Users\x\.codex\mp'` + "\n",
		"[plugins.\"visualize@local-openai\"]\nenabled = true\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}
}

func TestCodexConfigMarketplacesPrecedePlugins(t *testing.T) {
	got := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{{Name: "mp", Source: "/tmp/mp", SourceType: "local"}},
		Plugins:      []string{"p@mp"},
	})
	mIdx := strings.Index(got, "[marketplaces.mp]")
	pIdx := strings.Index(got, `[plugins."p@mp"]`)
	if mIdx < 0 || pIdx < 0 {
		t.Fatalf("both blocks expected:\n%s", got)
	}
	// A plugin selector whose marketplace is not yet declared resolves to nothing.
	if mIdx > pIdx {
		t.Fatalf("marketplace must be rendered before plugins:\n%s", got)
	}
}

func TestCodexConfigPluginOrderIsDeterministic(t *testing.T) {
	// The launch fingerprint hashes this text, so a map/slice ordering difference
	// would silently prevent process reuse across turns.
	a := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{
			{Name: "zeta", Source: "/z", SourceType: "local"},
			{Name: "alpha", Source: "/a", SourceType: "local"},
		},
		Plugins: []string{"b@alpha", "a@alpha"},
	})
	b := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{
			{Name: "alpha", Source: "/a", SourceType: "local"},
			{Name: "zeta", Source: "/z", SourceType: "local"},
		},
		Plugins: []string{"a@alpha", "b@alpha"},
	})
	if a != b {
		t.Fatalf("render must not depend on input order:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
	if strings.Index(a, "[marketplaces.alpha]") > strings.Index(a, "[marketplaces.zeta]") {
		t.Fatalf("marketplaces must be sorted by name:\n%s", a)
	}
}

func TestCodexConfigWithoutPluginsIsUnchanged(t *testing.T) {
	// The off state must be byte-identical to the pre-plugin config, so disabling
	// the feature cannot perturb a cached prefix.
	base := renderCodexConfig(codexConfig{ReasoningEffort: "high", DisableSubAgents: true})
	withEmpty := renderCodexConfig(codexConfig{
		ReasoningEffort:  "high",
		DisableSubAgents: true,
		Marketplaces:     []CodexMarketplace{},
		Plugins:          []string{},
	})
	if base != withEmpty {
		t.Fatalf("empty plugin config must render nothing extra:\n--- base ---\n%s\n--- with ---\n%s", base, withEmpty)
	}
}

func TestCodexMarketplaceGitRefRendered(t *testing.T) {
	got := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{
			{Name: "team", Source: "org/repo", SourceType: "git", Ref: "main"},
		},
	})
	if !strings.Contains(got, `source_type = "git"`) || !strings.Contains(got, `ref = "main"`) {
		t.Fatalf("git marketplace fields missing:\n%s", got)
	}
}

func TestCodexMarketplaceDefaultsToLocal(t *testing.T) {
	got := renderCodexConfig(codexConfig{
		Marketplaces: []CodexMarketplace{{Name: "mp", Source: "/tmp/mp"}},
	})
	if !strings.Contains(got, `source_type = "local"`) {
		t.Fatalf("missing source_type default:\n%s", got)
	}
}

func TestTomlLiteralStringFallsBackForQuotes(t *testing.T) {
	// A literal string cannot carry a single quote; the basic form escapes the
	// backslashes instead.
	got := tomlLiteralString(`C:\a'b`)
	if !strings.HasPrefix(got, `"`) || !strings.Contains(got, `\\a`) {
		t.Fatalf("expected escaped basic string, got %s", got)
	}
}

func TestCodexPluginFingerprintIgnoresOrder(t *testing.T) {
	a := codexPluginFingerprint(
		[]CodexMarketplace{{Name: "b", Source: "/b"}, {Name: "a", Source: "/a"}},
		[]string{"y@a", "x@a"},
	)
	b := codexPluginFingerprint(
		[]CodexMarketplace{{Name: "a", Source: "/a"}, {Name: "b", Source: "/b"}},
		[]string{"x@a", "y@a"},
	)
	if a != b {
		t.Fatalf("fingerprint must ignore ordering:\n%q\n%q", a, b)
	}
}

func TestCodexPluginFingerprintChangesWithSource(t *testing.T) {
	// Re-pointing a marketplace must re-provision the home, otherwise the stamp
	// would keep serving the old copy.
	a := codexPluginFingerprint([]CodexMarketplace{{Name: "a", Source: "/one"}}, nil)
	b := codexPluginFingerprint([]CodexMarketplace{{Name: "a", Source: "/two"}}, nil)
	if a == b {
		t.Fatal("fingerprint must change when a marketplace source changes")
	}
}

func TestCodexPluginAlreadyPresent(t *testing.T) {
	for _, msg := range []string{
		"Error: marketplace `mp` already added",
		"plugin already installed",
	} {
		if !codexPluginAlreadyPresent(msg) {
			t.Errorf("expected %q to read as already-present", msg)
		}
	}
	if codexPluginAlreadyPresent("Error: marketplace `openai-bundled` is reserved") {
		t.Error("a reserved-name failure is a real error, not already-present")
	}
}

func TestEnsureCodexPluginsSkipsWhenUnconfigured(t *testing.T) {
	// No configuration means the install step must not shell out at all: a bogus
	// binary path would fail loudly if it did.
	c := &CodexCLI{binPath: "definitely-not-a-real-binary"}
	if notes := c.ensureCodexPlugins(t.Context(), t.TempDir()); notes != nil {
		t.Fatalf("expected no work for an unconfigured provider, got %v", notes)
	}
}
