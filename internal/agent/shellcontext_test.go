package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// ShellToolsContextBlock is single-sourced and must NEVER return empty: when the
// gate is off (or no interpreter backs it) it has to state shell is disabled and
// carry the dead-tool rule, or the model loops a bare PowerShell/Bash call until
// the turn times out (FND-9c9a52aa, FND-6095a777, FND-e9c79d9a, FND-495575b8).
func TestShellToolsContextBlockDisabled(t *testing.T) {
	r := &Runtime{tun: NewTunables()}
	r.tun.SetShellEnabled(false) // gate off → shell unavailable regardless of the host

	got := r.ShellToolsContextBlock(false)
	if got == "" {
		t.Fatal("disabled block is empty; the agent would never learn shell is off")
	}
	for _, want := range []string{
		"DISABLED",
		"not enabled in this context", // the exact error the dead-tool rule keys on
		"do NOT repeat the identical", // no-retry rule
		"Read / Glob / Grep",          // the fallback tools it must switch to
	} {
		if !strings.Contains(got, want) {
			t.Errorf("disabled block missing %q\nblock: %s", want, got)
		}
	}
}

// When the gate is on AND the host has a backing interpreter, the block advertises
// the registered tools by their exact names (the enabled branch). Gated on
// ShellToolNames so the assertion stays deterministic on a shell-less host.
func TestShellToolsContextBlockEnabled(t *testing.T) {
	names := tools.ShellToolNames()
	if len(names) == 0 {
		t.Skip("no backing shell interpreter on this host; enabled branch unreachable")
	}
	r := &Runtime{tun: NewTunables()}
	r.tun.SetShellEnabled(true)

	got := r.ShellToolsContextBlock(false)
	if !strings.Contains(got, "ENABLED") {
		t.Errorf("enabled block does not announce ENABLED\nblock: %s", got)
	}
	if strings.Contains(got, "DISABLED") {
		t.Errorf("enabled block leaks the disabled wording\nblock: %s", got)
	}
	for _, n := range names {
		if !strings.Contains(got, n) {
			t.Errorf("enabled block omits registered tool %q\nblock: %s", n, got)
		}
	}
	// The confined flag flips the scope clause so it never contradicts the
	// working-directory block on an autonomous-confined turn.
	if unconfined := got; !strings.Contains(unconfined, "not confined to the working directory") {
		t.Errorf("unconfined block should say it is not confined\nblock: %s", unconfined)
	}
	confined := r.ShellToolsContextBlock(true)
	if !strings.Contains(confined, "confined to the working directory") ||
		strings.Contains(confined, "not confined to the working directory") {
		t.Errorf("confined block should state confinement\nblock: %s", confined)
	}
}

// The gate being on is not sufficient: if no interpreter backs it, the block must
// still fall to the DISABLED branch (prompt ↔ catalog parity — buildRegistry would
// not have registered a tool either). This asserts the len(names)==0 arm of the
// same condition without depending on the host having a shell.
func TestShellToolsContextBlockNeverEmpty(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		r := &Runtime{tun: NewTunables()}
		r.tun.SetShellEnabled(enabled)
		if r.ShellToolsContextBlock(false) == "" {
			t.Errorf("block empty with ShellEnabled=%v; must always state capability", enabled)
		}
	}
}
