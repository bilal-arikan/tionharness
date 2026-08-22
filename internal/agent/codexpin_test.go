package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestGuardedComplete_PinsCodexHome guards the asymmetry that made auxiliary
// codex turns (reflect/summary/title/insight) fail with a revoked login while
// tool-loop turns of the same agent worked: guardedComplete pinned only the
// claude home, so a codex-cli instance with an empty configDir never exported
// CODEX_HOME and the subprocess read the ambient ~/.codex instead. The
// observable proof on this path is the app-global home being resolved and
// created; the Complete call itself is expected to fail here (no codex binary /
// no login), which is irrelevant to what is asserted.
func TestGuardedComplete_PinsCodexHome(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	rt, _ := newTestRuntime(t, workDir)
	rt.providers.SetInstances([]providers.Instance{{ID: "codex-cli", KindID: "codex-cli"}})

	ctx := WithCallKind(context.Background(), KindReflect)
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Codex", Provider: "codex-cli", Model: "gpt-5"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// The turn may fail (no usable codex CLI in the test environment); the home
	// must be pinned before that attempt regardless.
	_, _ = rt.guardedComplete(ctx, agent, providers.Request{Model: "gpt-5"}, false)

	home := filepath.Join(rt.dataDir, "codex-home")
	if st, err := os.Stat(home); err != nil || !st.IsDir() {
		t.Fatalf("app-global codex home not created at %s: %v", home, err)
	}
}

// TestPinCodexHome_KeepsExplicitConfigDir: an instance that carries its own
// CODEX_HOME owns it, so pinning must not redirect it to the app-global home.
// The directory is still created, because CODEX_HOME must exist before the
// subprocess starts.
func TestPinCodexHome_KeepsExplicitConfigDir(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	dedicated := filepath.Join(t.TempDir(), "own-codex-home")

	cx := providers.NewCodexCLI("codex", "gpt-5", dedicated)
	if err := rt.PinCodexHome(cx); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if got := cx.ConfigDir(); got != dedicated {
		t.Fatalf("configDir = %q, want the instance's own %q", got, dedicated)
	}
	if st, err := os.Stat(dedicated); err != nil || !st.IsDir() {
		t.Fatalf("own codex home not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rt.dataDir, "codex-home")); !os.IsNotExist(err) {
		t.Fatalf("app-global home touched for an instance with its own home: %v", err)
	}
}

// A non-codex provider is a no-op, not an error.
func TestPinCodexHome_IgnoresOtherProviders(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	if err := rt.PinCodexHome(providers.NewClaudeCLI("claude", "", "", "", "")); err != nil {
		t.Fatalf("non-codex provider: %v", err)
	}
}

// TestPinCLIHome_PinsBothTransports: the single entry point every Complete site
// uses must pin whichever CLI home applies and stay a no-op for the other.
func TestPinCLIHome_PinsBothTransports(t *testing.T) {
	rt := &Runtime{dataDir: t.TempDir()}

	cx := providers.NewCodexCLI("codex", "gpt-5", "")
	if err := rt.PinCLIHome(cx); err != nil {
		t.Fatalf("PinCLIHome(codex): %v", err)
	}
	if cx.ConfigDir() != appCodexHomeDir(rt.dataDir) {
		t.Fatalf("codex home = %q, want %q", cx.ConfigDir(), appCodexHomeDir(rt.dataDir))
	}

	cc := providers.NewClaudeCLI("claude", "", "", "", "")
	if err := rt.PinCLIHome(cc); err != nil {
		t.Fatalf("PinCLIHome(claude): %v", err)
	}
	if cc.ConfigDir() != appCLIHomeDir(rt.dataDir, "claude-cli") {
		t.Fatalf("claude home = %q, want %q", cc.ConfigDir(), appCLIHomeDir(rt.dataDir, "claude-cli"))
	}
}
