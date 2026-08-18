package providers

import "testing"

// TestCodexCLIRegistryWiring verifies the codex-cli kind reaches the same
// registry seams claude-cli does: availability gated on the resolved binary,
// Get building the concrete provider, and an unconfigured path erroring out
// rather than returning a provider that would fail later at spawn time.
func TestCodexCLIRegistryWiring(t *testing.T) {
	r := NewRegistry("")
	r.codexCLIPath = "" // force-clear whatever PATH detection found on this host

	if r.Available("codex-cli") {
		t.Error("codex-cli reported available with no binary")
	}
	if r.CodexCLIAvailable() {
		t.Error("CodexCLIAvailable true with no binary")
	}
	if _, err := r.Get("codex-cli"); err == nil {
		t.Error("Get(codex-cli): want unconfigured error, got nil")
	}

	r.codexCLIPath = "/usr/bin/codex" // simulate CLI present
	if !r.Available("codex-cli") {
		t.Error("codex-cli not available after path set")
	}
	if got := r.CodexCLIPath(); got != "/usr/bin/codex" {
		t.Errorf("CodexCLIPath() = %q, want /usr/bin/codex", got)
	}
	p, err := r.Get("codex-cli")
	if err != nil {
		t.Fatalf("Get(codex-cli): %v", err)
	}
	if p.Name() != "codex-cli" {
		t.Errorf("Get(codex-cli).Name() = %q, want codex-cli", p.Name())
	}
}

// TestCodexConfigDirReachesResolvedConfig checks the CODEX_HOME override set
// from settings actually lands in the config a kind builds from. Without this
// the setting would persist and display correctly while the subprocess silently
// kept using the ambient ~/.codex.
func TestCodexConfigDirReachesResolvedConfig(t *testing.T) {
	r := NewRegistry("")
	r.SetCodexConfigDir("/tmp/codex-home")
	if got := r.resolve("codex-cli").CodexConfigDir; got != "/tmp/codex-home" {
		t.Errorf("resolve().CodexConfigDir = %q, want /tmp/codex-home", got)
	}
}

// TestSetCodexCLIPathEmptyRestoresAutoDetect mirrors SetClaudeCLIPath: clearing
// the override must fall back to PATH lookup, not pin an empty path that would
// make the provider permanently unavailable.
func TestSetCodexCLIPathEmptyRestoresAutoDetect(t *testing.T) {
	r := NewRegistry("")
	autodetected := r.CodexCLIPath() // whatever this host resolves (may be "")

	r.SetCodexCLIPath("/custom/codex")
	if got := r.CodexCLIPath(); got != "/custom/codex" {
		t.Fatalf("CodexCLIPath() = %q, want /custom/codex", got)
	}
	r.SetCodexCLIPath("")
	if got := r.CodexCLIPath(); got != autodetected {
		t.Errorf("CodexCLIPath() after clearing = %q, want auto-detected %q", got, autodetected)
	}
}
