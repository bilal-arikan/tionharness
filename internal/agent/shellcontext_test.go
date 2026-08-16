package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// shellCtxRuntime builds a runtime backed by a real store (toolFilter reads the
// workspace tool config) plus an agent carrying the given allowlist. An empty
// allow slice leaves AllowedTools unset — "allow everything".
func shellCtxRuntime(t *testing.T, shellEnabled bool, allow []string) (*Runtime, db.Agent, context.Context) {
	t.Helper()
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetShellEnabled(shellEnabled)
	ctx := context.Background()
	a := db.Agent{Name: "ShellCtx", Provider: "anthropic"}
	if len(allow) > 0 {
		b, err := json.Marshal(allow)
		if err != nil {
			t.Fatalf("marshal allowlist: %v", err)
		}
		a.AllowedTools = string(b)
	}
	agent, err := rt.db.CreateAgent(ctx, a)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return rt, agent, ctx
}

// ShellToolsContextBlock is single-sourced and must NEVER return empty: when the
// gate is off (or no interpreter backs it) it has to state shell is disabled and
// carry the dead-tool rule, or the model loops a bare PowerShell/Bash call until
// the turn times out (FND-9c9a52aa, FND-6095a777, FND-e9c79d9a, FND-495575b8).
func TestShellToolsContextBlockDisabled(t *testing.T) {
	rt, agent, ctx := shellCtxRuntime(t, false, nil) // gate off → shell unavailable regardless of the host

	got := rt.ShellToolsContextBlock(ctx, agent, false)
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
	rt, agent, ctx := shellCtxRuntime(t, true, nil)

	got := rt.ShellToolsContextBlock(ctx, agent, false)
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
	confined := rt.ShellToolsContextBlock(ctx, agent, true)
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
		rt, agent, ctx := shellCtxRuntime(t, enabled, nil)
		if rt.ShellToolsContextBlock(ctx, agent, false) == "" {
			t.Errorf("block empty with ShellEnabled=%v; must always state capability", enabled)
		}
	}
}

// The workspace gate is NOT the whole truth: an agent whose allowlist omits both
// shell tools cannot call either, no matter how the gate is set. Telling it shell
// is ENABLED made it call Bash and collect "No such tool available" on every
// attempt — the block must report the DISABLED variant instead.
func TestShellToolsContextBlockAllowlistWithoutShell(t *testing.T) {
	if len(tools.ShellToolNames()) == 0 {
		t.Skip("no backing shell interpreter on this host; the gate alone would say DISABLED anyway")
	}
	// A read-only profile allowlist: file tools only, no Bash/PowerShell.
	rt, agent, ctx := shellCtxRuntime(t, true, []string{"Read", "LS", "Glob", "Grep"})

	got := rt.ShellToolsContextBlock(ctx, agent, false)
	if strings.Contains(got, "ENABLED") {
		t.Errorf("agent without shell in its allowlist was told shell is ENABLED\nblock: %s", got)
	}
	if !strings.Contains(got, "DISABLED") {
		t.Errorf("block must state shell is DISABLED for this agent\nblock: %s", got)
	}
}

// A partial allowlist must advertise ONLY the shell tool it actually permits: an
// agent allowed Bash but not PowerShell may not be told PowerShell is callable.
func TestShellToolsContextBlockAllowlistPartialShell(t *testing.T) {
	names := tools.ShellToolNames()
	if len(names) < 2 {
		t.Skip("host backs fewer than two shell tools; the partial case is unreachable")
	}
	allowed, excluded := names[0], names[1]
	rt, agent, ctx := shellCtxRuntime(t, true, []string{"Read", allowed})

	got := rt.ShellToolsContextBlock(ctx, agent, false)
	if !strings.Contains(got, "ENABLED") {
		t.Fatalf("agent allowed %q should still be told shell is ENABLED\nblock: %s", allowed, got)
	}
	if !strings.Contains(got, allowed) {
		t.Errorf("block omits the permitted shell tool %q\nblock: %s", allowed, got)
	}
	if strings.Contains(got, excluded) {
		t.Errorf("block advertises %q, which the allowlist strips\nblock: %s", excluded, got)
	}
}

// The agent DENYLIST is the user-facing restriction (the allowlist above is the
// legacy profile one) and must gate the block identically — "PowerShell disabled
// to force Bash" is a real workspace configuration.
func TestShellToolsContextBlockDenylistBlocksShell(t *testing.T) {
	names := tools.ShellToolNames()
	if len(names) == 0 {
		t.Skip("no backing shell interpreter on this host")
	}
	rt, _, ctx := shellCtxRuntime(t, true, nil)
	blocked, err := json.Marshal(names) // deny every shell tool the host backs
	if err != nil {
		t.Fatalf("marshal denylist: %v", err)
	}
	agent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "NoShell", Provider: "anthropic", BlockedTools: string(blocked),
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got := rt.ShellToolsContextBlock(ctx, agent, false)
	if strings.Contains(got, "ENABLED") {
		t.Errorf("agent with every shell tool on its denylist was told shell is ENABLED\nblock: %s", got)
	}
}
