package providers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// findProviderLedgerEntry returns the newest default-ledger entry whose command
// line contains marker. The default ledger is process-wide, so the marker has to
// be specific enough to belong to this test alone.
func findProviderLedgerEntry(t *testing.T, marker string) (procwatch.Entry, bool) {
	t.Helper()
	for _, e := range procwatch.Default().List(procwatch.Filter{Kinds: []procwatch.Kind{procwatch.KindProvider}}) {
		if strings.Contains(e.Command, marker) {
			return e, true
		}
	}
	return procwatch.Entry{}, false
}

// TestCompactNativeRegistersAppServerInTheLedger pins the instrumentation of the
// longest-lived process this package starts outside a turn transport: the codex
// app-server that native compaction drives over stdio. It used to run completely
// untracked — a wedged one held the turn with nothing in the process panel to
// stop or even see.
//
// The binary deliberately does not exist, so the registration is asserted on the
// failure path: the entry has to exist (Begin happens BEFORE Start) and carry the
// full command line, the turn's owner INCLUDING its parent session, and a
// terminal status. A Begin moved after a successful Start would leave this exact
// case — the one an operator debugs — invisible again.
func TestCompactNativeRegistersAppServerInTheLedger(t *testing.T) {
	base := t.TempDir()
	const threadID = "019c-ledger-thread"
	scope := strings.Join([]string{"SESLEDGERCOMPACT", "AG1", "codex-cli", "gpt-5-codex", "static system prefix"}, "\x00")
	seedCodexThread(t, base, scope, threadID)

	// A unique, absent binary name: it is the ledger marker AND what makes Start
	// fail without waiting on a real app-server.
	bin := filepath.Join(t.TempDir(), "codex-appserver-ledger-probe")
	c := NewCodexCLI(bin, "", base)
	ctx := procwatch.WithOwner(t.Context(), procwatch.Owner{
		WorkspaceID:     "WSLEDGER",
		SessionID:       "SESLEDGERCOMPACT",
		ParentSessionID: "SESLEDGERCOORD",
	})
	if _, err := c.CompactNative(ctx, threadID, Request{CLIResumeScope: scope}); err == nil {
		t.Fatal("CompactNative succeeded against a binary that does not exist")
	}

	e, ok := findProviderLedgerEntry(t, "codex-appserver-ledger-probe")
	if !ok {
		t.Fatal("the codex app-server never reached the ledger")
	}
	if !strings.Contains(e.Command, "app-server --listen stdio:// --strict-config") {
		t.Errorf("command = %q, want the full app-server invocation", e.Command)
	}
	if e.Label != "codex-cli (app-server)" {
		t.Errorf("label = %q, want the app-server label", e.Label)
	}
	if e.Owner.SessionID != "SESLEDGERCOMPACT" || e.Owner.ParentSessionID != "SESLEDGERCOORD" || e.Owner.WorkspaceID != "WSLEDGER" {
		t.Errorf("owner = %+v, want the owner stamped on the turn context", e.Owner)
	}
	if e.Status == procwatch.StatusRunning || e.EndedAt == 0 {
		t.Errorf("status = %q, want a terminal one — a process that never finishes in the ledger stays in the panel forever", e.Status)
	}
	if e.Error == "" {
		t.Error("no error recorded — the panel would show a failure with no explanation")
	}
}
