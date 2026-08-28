package tools

import (
	"sort"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// wantNameOnlyDefaults is the verbatim name-only list moved out of
// buildRegistry (internal/agent/toolsetup.go). It is duplicated here on purpose:
// the table is the thing under test, so the expectation must be written
// independently of it.
var wantNameOnlyDefaults = []string{
	"apply_patch", "archive_sessions", "config_validate", "conversation_search",
	"deactivate_tools", "delete_lesson", "expand", "focus_view",
	"get_session_info", "handoff_session", "insight_apply_finding",
	"insight_list_findings", "insight_scan", "list_sessions", "mermaid_validate",
	"notify", "read_lessons", "read_session_debug", "render_template",
	"send_message", "shell_manage", "skill_validate", "update_artifact",
	"update_session", "update_user_preferences",
}

var wantHiddenDefaults = []string{"list_config", "read_config", "secret", "write_config"}

func namesWithTier(m map[string]string, tier string) []string {
	var out []string
	for name, v := range m {
		if v == tier {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func TestDefaultToolTiersTableContent(t *testing.T) {
	d := DefaultTiers()

	gotNameOnly := namesWithTier(d.Tool, VisibilityNameOnly)
	if len(gotNameOnly) != len(wantNameOnlyDefaults) {
		t.Fatalf("name-only default count = %d, want %d\ngot:  %v\nwant: %v",
			len(gotNameOnly), len(wantNameOnlyDefaults), gotNameOnly, wantNameOnlyDefaults)
	}
	for i, name := range wantNameOnlyDefaults {
		if gotNameOnly[i] != name {
			t.Errorf("name-only default[%d] = %q, want %q", i, gotNameOnly[i], name)
		}
	}

	gotHidden := namesWithTier(d.Tool, VisibilityHidden)
	if len(gotHidden) != len(wantHiddenDefaults) {
		t.Fatalf("hidden default count = %d, want %d (got %v)", len(gotHidden), len(wantHiddenDefaults), gotHidden)
	}
	for i, name := range wantHiddenDefaults {
		if gotHidden[i] != name {
			t.Errorf("hidden default[%d] = %q, want %q", i, gotHidden[i], name)
		}
	}
}

func TestDefaultTiersUseKnownVisibilityConstants(t *testing.T) {
	known := map[string]bool{
		VisibilityFull: true, VisibilitySummary: true,
		VisibilityNameOnly: true, VisibilityHidden: true,
	}
	d := DefaultTiers()
	for name, tier := range d.Tool {
		if !known[tier] {
			t.Errorf("tool %q has unknown tier %q", name, tier)
		}
	}
	for key, tier := range d.Bundle {
		if !known[tier] {
			t.Errorf("bundle %q has unknown tier %q", key, tier)
		}
		if !ValidBundleKey(key) {
			t.Errorf("bundle key %q is not a valid bundle key", key)
		}
	}
}

func TestDefaultBundleTiersOnlyMCPWildcard(t *testing.T) {
	d := DefaultTiers()
	if len(d.Bundle) != 1 {
		t.Fatalf("bundle defaults = %v, want exactly the mcp wildcard row", d.Bundle)
	}
	if d.Bundle[MCPBundleWildcard] != VisibilityNameOnly {
		t.Errorf("%s = %q, want %q", MCPBundleWildcard, d.Bundle[MCPBundleWildcard], VisibilityNameOnly)
	}
	// No built-in group may carry a default today: any value would change behavior.
	for _, cat := range Categories() {
		if tier := d.Bundle[GroupPrefix+cat]; tier != "" {
			t.Errorf("group:%s must have no default tier, got %q", cat, tier)
		}
	}
}

func TestTierForPrecedence(t *testing.T) {
	d := DefaultTiers()
	if got := d.TierFor("apply_patch"); got != VisibilityNameOnly {
		t.Errorf("TierFor(apply_patch) = %q, want %q", got, VisibilityNameOnly)
	}
	if got := d.TierFor("secret"); got != VisibilityHidden {
		t.Errorf("TierFor(secret) = %q, want %q", got, VisibilityHidden)
	}
	// Any MCP tool resolves through the wildcard bundle row.
	if got := d.TierFor("playwright__click"); got != VisibilityNameOnly {
		t.Errorf("TierFor(playwright__click) = %q, want %q", got, VisibilityNameOnly)
	}
	// A built-in with no row keeps the registration default (no opinion).
	if got := d.TierFor("Read"); got != "" {
		t.Errorf("TierFor(Read) = %q, want \"\" (inherit)", got)
	}
	// The per-tool layer wins over a bundle row.
	d.Bundle[MCPBundlePrefix+"srv"] = VisibilityHidden
	d.Tool["srv__loud"] = VisibilityFull
	if got := d.TierFor("srv__loud"); got != VisibilityFull {
		t.Errorf("TierFor(srv__loud) = %q, want tool entry %q", got, VisibilityFull)
	}
	if got := d.TierFor("srv__quiet"); got != VisibilityHidden {
		t.Errorf("TierFor(srv__quiet) = %q, want server row %q", got, VisibilityHidden)
	}
	// DefaultTiers hands out a copy: the mutations above must not leak.
	if fresh := DefaultTiers(); fresh.Tool["srv__loud"] != "" || fresh.Bundle[MCPBundlePrefix+"srv"] != "" {
		t.Error("DefaultTiers must return a copy, not the package-level tables")
	}
}

func TestApplyToolDefaultsStampsUnregisteredNames(t *testing.T) {
	reg := NewRegistry()
	reg.ApplyToolDefaults(DefaultTiers())

	// R1: the stamp is by NAME, unconditional — VisibilityOf classifies bridged
	// names that are not among this registry's builtins.
	if got := reg.VisibilityOf("apply_patch"); got != VisibilityNameOnly {
		t.Errorf("VisibilityOf(apply_patch) = %q, want %q", got, VisibilityNameOnly)
	}
	if got := reg.VisibilityOf("secret"); got != VisibilityHidden {
		t.Errorf("VisibilityOf(secret) = %q, want %q", got, VisibilityHidden)
	}
	if got := reg.VisibilityOf("Read"); got != VisibilityFull {
		t.Errorf("VisibilityOf(Read) = %q, want %q (untouched)", got, VisibilityFull)
	}
}

func TestApplyToolDefaultsKeepsSelfManagedStamp(t *testing.T) {
	reg := NewRegistry()
	reg.MarkHidden("send_message", "handoff_session")
	reg.MarkSelfManaged("send_message", "handoff_session")
	reg.ApplyToolDefaults(DefaultTiers())

	for _, name := range []string{"send_message", "handoff_session"} {
		if got := reg.VisibilityOf(name); got != VisibilityNameOnly {
			t.Errorf("VisibilityOf(%s) = %q, want %q", name, got, VisibilityNameOnly)
		}
		if !reg.IsSelfManaged(name) {
			t.Errorf("%s lost its self-managed stamp; the claude-cli bridge would drop it", name)
		}
	}
}

func TestApplyBundleDefaultsCoversKnownNamesOnly(t *testing.T) {
	reg := NewRegistry()
	reg.Add(NewTodoWriteTool())
	reg.AttachMCP([]mcp.CatalogEntry{{Server: "srv", NamespacedName: "srv__do"}}, nil, nil)

	d := TierDefaults{
		Bundle: map[string]string{GroupPrefix + CategoryAutomation: VisibilityHidden},
		Tool:   map[string]string{},
	}
	reg.ApplyBundleDefaults(d)
	if got := reg.VisibilityOf("todo_write"); got != VisibilityHidden {
		t.Errorf("VisibilityOf(todo_write) = %q, want %q", got, VisibilityHidden)
	}
	// create_flow is in the same category but not registered here: bundle defaults
	// only cover enumerable members.
	if got := reg.VisibilityOf("create_flow"); got != VisibilityFull {
		t.Errorf("VisibilityOf(create_flow) = %q, want %q (not a member of this registry)", got, VisibilityFull)
	}
	// A per-tool entry wins over the bundle row.
	d.Tool["todo_write"] = VisibilityFull
	reg2 := NewRegistry()
	reg2.Add(NewTodoWriteTool())
	reg2.ApplyBundleDefaults(d)
	if got := reg2.VisibilityOf("todo_write"); got != VisibilityFull {
		t.Errorf("VisibilityOf(todo_write) = %q, want the tool entry to win (%q)", got, VisibilityFull)
	}
}
