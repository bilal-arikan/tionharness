package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// profileAgent mirrors what resolveWorkerTarget materializes for a built-in
// profile: a persisted agent whose legacy allowlist pins it to the profile's work
// tools. It is the shape that made the coordination surface disappear.
func profileAgent(t *testing.T, allowed ...string) db.Agent {
	t.Helper()
	raw, err := json.Marshal(allowed)
	if err != nil {
		t.Fatalf("marshal allowlist: %v", err)
	}
	return db.Agent{Name: "worker:explore", MCPEnabled: true, AllowedTools: string(raw)}
}

// TestToolFilterExemptsCoordinationFromAllowlist is the regression for the live
// failure: a profile-restricted agent driving a sub-coordinator session kept its
// narrow WORK allowlist but lost the whole coordination surface, so it could
// neither spawn nor report — and, because CLI-native file tools bypass this
// filter entirely, it silently did the work itself instead of failing.
func TestToolFilterExemptsCoordinationFromAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	filter := rt.toolFilter(context.Background(), profileAgent(t, "Read", "LS", "Glob", "Grep", "WebFetch"))
	if filter == nil {
		t.Fatal("an agent with an allowlist must produce a filter")
	}

	for _, name := range tools.CoordinationToolNames {
		if !filter(name) {
			t.Errorf("%s must survive a profile allowlist — it is gated by the session, not the agent", name)
		}
	}
	// The allowlist must still do its actual job on WORK tools, or the exemption
	// has quietly become "profiles are unrestricted".
	if !filter("Read") {
		t.Error("Read is on the allowlist and must stay available")
	}
	for _, name := range []string{"Write", "Edit", "Bash"} {
		if filter(name) {
			t.Errorf("%s is not on the allowlist and must stay unavailable", name)
		}
	}
}

// TestToolFilterCoordinationStillBlockable proves the exemption is scoped to the
// ALLOWLIST. An explicit per-agent denylist is a deliberate user decision about
// this agent and must still win — otherwise "block spawn_worker" would silently
// stop working.
func TestToolFilterCoordinationStillBlockable(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	a := profileAgent(t, "Read")
	a.BlockedTools = `["spawn_worker"]`
	filter := rt.toolFilter(context.Background(), a)
	if filter == nil {
		t.Fatal("expected a filter")
	}
	if filter("spawn_worker") {
		t.Error("an explicitly blocked coordination tool must stay blocked")
	}
	if !filter("report_to_coordinator") {
		t.Error("blocking one coordination tool must not blanket-block the surface")
	}
}

// TestCoordinationToolNamesMatchBridge keeps the exemption list honest: a tool the
// CLI bridge dispatches but IsCoordinationTool does not know would be filtered
// back out by a profile allowlist — reintroducing the exact bug, for that one
// tool, with nothing to catch it.
func TestCoordinationToolNamesMatchBridge(t *testing.T) {
	funcs := &tools.CoordinationFuncs{}
	for _, name := range tools.CoordinationToolNames {
		// Args are deliberately invalid: only the "was it recognised" bool matters,
		// and every one of these tools rejects an empty object.
		if _, handled, _ := dispatchCoordinationBridge(context.Background(), funcs, name, json.RawMessage(`{}`)); !handled {
			t.Errorf("%s is listed as a coordination tool but the CLI bridge does not dispatch it", name)
		}
	}
	if _, handled, _ := dispatchCoordinationBridge(context.Background(), funcs, "Read", json.RawMessage(`{}`)); handled {
		t.Error("dispatchCoordinationBridge claimed a non-coordination tool")
	}
}
