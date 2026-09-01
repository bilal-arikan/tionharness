package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// goldenTiersAuto is the frozen name -> visibility tier table for a WRITE-CAPABLE
// agent (PermissionMode "auto"), captured from buildRegistry on main before the
// bundle-default refactor. It is the tripwire for that refactor: moving the
// hand-written MarkNameOnly/MarkHidden blocks in toolsetup.go into a data layer
// must leave every entry below byte-identical. A required edit here means the
// move changed behaviour — stop and diff instead of updating the table.
//
// Regenerate (only when a tier change is INTENDED) with:
//
//	TIER_GOLDEN_DUMP=1 go test ./internal/agent/ -run TierParity -v
var goldenTiersAuto = map[string]string{
	"Edit":                  "full",
	"Glob":                  "full",
	"Grep":                  "full",
	"LS":                    "full",
	"Read":                  "full",
	"WebFetch":              "full",
	"WebSearch":             "full",
	"Write":                 "full",
	"activate_tools":        "full",
	"apply_patch":           "name-only",
	"archive_sessions":      "name-only",
	"ask_user":              "full",
	"config_validate":       "name-only",
	"conversation_search":   "name-only",
	"create_agent":          "hidden",
	"create_artifact":       "full",
	"create_automation":     "hidden",
	"create_flow":           "hidden",
	"create_hook":           "hidden",
	"create_mcp_server":     "hidden",
	"create_schedule":       "hidden",
	"create_skill":          "hidden",
	"create_task":           "hidden",
	"get_task":              "hidden",
	"deactivate_tools":      "name-only",
	"delete_agent":          "hidden",
	"delete_artifact":       "hidden",
	"delete_automation":     "hidden",
	"delete_flow":           "hidden",
	"delete_hook":           "hidden",
	"delete_lesson":         "name-only",
	"delete_mcp_server":     "hidden",
	"delete_schedule":       "hidden",
	"delete_skill":          "hidden",
	"delete_task":           "hidden",
	"deliver_flow_input":    "hidden",
	"expand":                "name-only",
	"focus_view":            "name-only",
	"get_flow":              "hidden",
	"get_session_info":      "name-only",
	"get_view":              "full",
	"handoff_session":       "name-only",
	"import_skill":          "hidden",
	"insight_apply_finding": "name-only",
	"insight_list_findings": "name-only",
	"insight_scan":          "name-only",
	"list_agents":           "hidden",
	"list_artifacts":        "hidden",
	"list_automations":      "hidden",
	"list_config":           "hidden",
	"list_flow_runs":        "hidden",
	"list_flows":            "hidden",
	"list_hooks":            "hidden",
	"list_mcp_servers":      "hidden",
	"list_providers":        "hidden",
	"list_schedules":        "hidden",
	"list_sessions":         "name-only",
	"list_tasks":            "hidden",
	"mermaid_validate":      "name-only",
	"move_task":             "hidden",
	"set_archived_task":     "hidden",
	"notify":                "name-only",
	"read_artifact":         "hidden",
	"read_config":           "hidden",
	"read_lessons":          "name-only",
	"read_logs":             "hidden",
	"read_session_debug":    "name-only",
	"render_template":       "name-only",
	"request_confirmation":  "full",
	"run_flow":              "hidden",
	"run_schedule":          "hidden",
	"run_subagent":          "full",
	"schedule_wake":         "full",
	"send_message":          "name-only",
	"skill_search":          "full",
	"skill_validate":        "name-only",
	"todo_write":            "full",
	"toggle_mcp_server":     "hidden",
	"tool_search":           "full",
	"update_agent":          "hidden",
	"update_artifact":       "name-only",
	"update_automation":     "hidden",
	"update_flow":           "hidden",
	"update_hook":           "hidden",
	"update_schedule":       "hidden",
	"update_session":        "name-only",
	"update_skill":          "hidden",
	"update_task":           "hidden",
	"use_skill":             "full",
	"write_config":          "hidden",
}

// goldenTiersReadOnly is the same table for a READ-ONLY agent. It differs from
// goldenTiersAuto only where toolsetup.go's role-aware trim applies
// (MarkLazy("Write","Edit") for read-only agents).
var goldenTiersReadOnly = map[string]string{
	"Edit":                  "summary", // demoted by the read-only trim
	"Glob":                  "full",
	"Grep":                  "full",
	"LS":                    "full",
	"Read":                  "full",
	"WebFetch":              "full",
	"WebSearch":             "full",
	"Write":                 "summary", // demoted by the read-only trim
	"activate_tools":        "full",
	"apply_patch":           "name-only",
	"archive_sessions":      "name-only",
	"ask_user":              "full",
	"config_validate":       "name-only",
	"conversation_search":   "name-only",
	"create_agent":          "hidden",
	"create_artifact":       "full",
	"create_automation":     "hidden",
	"create_flow":           "hidden",
	"create_hook":           "hidden",
	"create_mcp_server":     "hidden",
	"create_schedule":       "hidden",
	"create_skill":          "hidden",
	"create_task":           "hidden",
	"get_task":              "hidden",
	"deactivate_tools":      "name-only",
	"delete_agent":          "hidden",
	"delete_artifact":       "hidden",
	"delete_automation":     "hidden",
	"delete_flow":           "hidden",
	"delete_hook":           "hidden",
	"delete_lesson":         "name-only",
	"delete_mcp_server":     "hidden",
	"delete_schedule":       "hidden",
	"delete_skill":          "hidden",
	"delete_task":           "hidden",
	"deliver_flow_input":    "hidden",
	"expand":                "name-only",
	"focus_view":            "name-only",
	"get_flow":              "hidden",
	"get_session_info":      "name-only",
	"get_view":              "full",
	"handoff_session":       "name-only",
	"import_skill":          "hidden",
	"insight_apply_finding": "name-only",
	"insight_list_findings": "name-only",
	"insight_scan":          "name-only",
	"list_agents":           "hidden",
	"list_artifacts":        "hidden",
	"list_automations":      "hidden",
	"list_config":           "hidden",
	"list_flow_runs":        "hidden",
	"list_flows":            "hidden",
	"list_hooks":            "hidden",
	"list_mcp_servers":      "hidden",
	"list_providers":        "hidden",
	"list_schedules":        "hidden",
	"list_sessions":         "name-only",
	"list_tasks":            "hidden",
	"mermaid_validate":      "name-only",
	"move_task":             "hidden",
	"set_archived_task":     "hidden",
	"notify":                "name-only",
	"read_artifact":         "hidden",
	"read_config":           "hidden",
	"read_lessons":          "name-only",
	"read_logs":             "hidden",
	"read_session_debug":    "name-only",
	"render_template":       "name-only",
	"request_confirmation":  "full",
	"run_flow":              "hidden",
	"run_schedule":          "hidden",
	"run_subagent":          "full",
	"schedule_wake":         "full",
	"send_message":          "name-only",
	"skill_search":          "full",
	"skill_validate":        "name-only",
	"todo_write":            "full",
	"toggle_mcp_server":     "hidden",
	"tool_search":           "full",
	"update_agent":          "hidden",
	"update_artifact":       "name-only",
	"update_automation":     "hidden",
	"update_flow":           "hidden",
	"update_hook":           "hidden",
	"update_schedule":       "hidden",
	"update_session":        "name-only",
	"update_skill":          "hidden",
	"update_task":           "hidden",
	"use_skill":             "full",
	"write_config":          "hidden",
}

// goldenSelfManaged is the set of tools carrying the MarkSelfManaged membership
// stamp. The stamp is independent of the visibility tier and load-bearing for the
// claude-cli bridge (BridgeableDefsFiltered keeps advertising a self-management
// tool after a tier promotion clears its lazy flag), so a refactor that only
// preserves tiers would still be a regression if this set shrank. Notably
// send_message / handoff_session appear here AND carry the name-only tier.
var goldenSelfManaged = []string{
	"create_agent",
	"create_automation",
	"create_flow",
	"create_hook",
	"create_mcp_server",
	"create_schedule",
	"create_skill",
	"create_task",
	"get_task",
	"delete_agent",
	"delete_artifact",
	"delete_automation",
	"delete_flow",
	"delete_hook",
	"delete_mcp_server",
	"delete_schedule",
	"delete_skill",
	"delete_task",
	"deliver_flow_input",
	"get_flow",
	"handoff_session", // stamped self-managed AND promoted to the name-only tier
	"import_skill",
	"list_agents",
	"list_artifacts",
	"list_automations",
	"list_flow_runs",
	"list_flows",
	"list_hooks",
	"list_mcp_servers",
	"list_providers",
	"list_schedules",
	"list_tasks",
	"move_task",
	"set_archived_task",
	"read_artifact",
	"read_logs",
	"run_flow",
	"run_schedule",
	"send_message", // stamped self-managed AND promoted to the name-only tier
	"toggle_mcp_server",
	"update_agent",
	"update_automation",
	"update_flow",
	"update_hook",
	"update_schedule",
	"update_skill",
	"update_task",
}

// envGatedTools are built-ins whose REGISTRATION depends on the host (an
// interpreter/shell being present, a sandbox being ready). Their presence is not
// portable across developer machines and CI, so the golden comparison checks
// their tier only when they are actually registered, and never fails on their
// absence. Everything else must match the table exactly.
var envGatedTools = map[string]bool{
	// Shell family: registered only when the shell tunable is on AND the backing
	// shell exists (Unix→Bash, Windows→PowerShell), so which of the two appears
	// is host-dependent.
	"Bash":           true,
	"PowerShell":     true,
	"shell_manage":   true,
	"transform_data": true,
	// Code-execution mode: needs the shell tunable, the code-mode tunable and a
	// ready sandbox.
	"run_code": true,
	// Confined workspace-config tools: only when the config sandbox is ready.
	"read_config":  true,
	"write_config": true,
	"list_config":  true,
	// Vault-backed; only when a secret store exists.
	"secret": true,
	// Session-context/preferences family: dependency-gated in selfManageBuiltins.
	"update_user_preferences": true,
}

// tierCensus collects the effective visibility tier of every registered built-in
// tool for one agent, straight out of the production buildRegistry path.
func tierCensus(t *testing.T, rt *Runtime, agent db.Agent) map[string]string {
	t.Helper()
	reg := rt.buildRegistry(context.Background(), agent)
	out := map[string]string{}
	for _, d := range reg.BuiltinDefs(nil) {
		out[d.Name] = reg.VisibilityOf(d.Name)
	}
	return out
}

// compareTiers diffs an observed census against a golden table and reports the
// three failure kinds separately, so a diff says WHAT moved rather than just
// "not equal".
func compareTiers(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	var missing, extra, changed []string
	for name, wantTier := range want {
		gotTier, ok := got[name]
		if !ok {
			if envGatedTools[name] {
				continue // host-gated: absence is not a regression
			}
			missing = append(missing, name+" (want "+wantTier+")")
			continue
		}
		if gotTier != wantTier {
			changed = append(changed, fmt.Sprintf("%s: %s -> %s", name, wantTier, gotTier))
		}
	}
	for name, gotTier := range got {
		if _, ok := want[name]; !ok && !envGatedTools[name] {
			extra = append(extra, name+" (is "+gotTier+")")
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(changed)

	if len(missing) > 0 {
		t.Errorf("[%s] %d tool(s) in the golden table are NO LONGER REGISTERED:\n  %s",
			label, len(missing), strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("[%s] %d NEW tool(s) not in the golden table — add them with their intended tier:\n  %s",
			label, len(extra), strings.Join(extra, "\n  "))
	}
	if len(changed) > 0 {
		t.Errorf("[%s] %d tool(s) CHANGED TIER (golden -> observed):\n  %s",
			label, len(changed), strings.Join(changed, "\n  "))
	}
}

// dumpGolden prints a paste-ready Go literal of a census. Only runs under
// TIER_GOLDEN_DUMP, so a normal test run stays silent.
func dumpGolden(t *testing.T, varName string, census map[string]string) {
	t.Helper()
	if os.Getenv("TIER_GOLDEN_DUMP") == "" {
		return
	}
	names := make([]string, 0, len(census))
	for n := range census {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "var %s = map[string]string{\n", varName)
	for _, n := range names {
		fmt.Fprintf(&b, "\t%q: %q,\n", n, census[n])
	}
	b.WriteString("}\n")
	t.Logf("GOLDEN DUMP\n%s", b.String())
}

// TestTierParityGoldenAuto freezes the visibility tiers a write-capable agent
// gets from buildRegistry today.
func TestTierParityGoldenAuto(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	census := tierCensus(t, rt, db.Agent{ID: "auto", MCPEnabled: true, PermissionMode: "auto"})
	dumpGolden(t, "goldenTiersAuto", census)
	compareTiers(t, "auto", census, goldenTiersAuto)
}

// TestTierParityGoldenReadOnly freezes the tiers a read-only agent gets, i.e. the
// branch where Write/Edit are demoted to the load-on-demand catalog.
func TestTierParityGoldenReadOnly(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	census := tierCensus(t, rt, db.Agent{ID: "ro", MCPEnabled: true, PermissionMode: "read-only"})
	dumpGolden(t, "goldenTiersReadOnly", census)
	compareTiers(t, "read-only", census, goldenTiersReadOnly)
}

// TestTierParitySelfManagedStamp freezes the MarkSelfManaged membership set.
func TestTierParitySelfManagedStamp(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	reg := rt.buildRegistry(context.Background(), db.Agent{ID: "auto", MCPEnabled: true, PermissionMode: "auto"})
	var got []string
	for _, d := range reg.BuiltinDefs(nil) {
		if reg.IsSelfManaged(d.Name) {
			got = append(got, d.Name)
		}
	}
	sort.Strings(got)
	if os.Getenv("TIER_GOLDEN_DUMP") != "" {
		var b strings.Builder
		b.WriteString("var goldenSelfManaged = []string{\n")
		for _, n := range got {
			fmt.Fprintf(&b, "\t%q,\n", n)
		}
		b.WriteString("}\n")
		t.Logf("GOLDEN DUMP\n%s", b.String())
	}

	wantSet := map[string]bool{}
	for _, n := range goldenSelfManaged {
		wantSet[n] = true
	}
	gotSet := map[string]bool{}
	for _, n := range got {
		gotSet[n] = true
	}
	var lost, added []string
	for _, n := range goldenSelfManaged {
		if !gotSet[n] && !envGatedTools[n] {
			lost = append(lost, n)
		}
	}
	for _, n := range got {
		if !wantSet[n] && !envGatedTools[n] {
			added = append(added, n)
		}
	}
	if len(lost) > 0 {
		t.Errorf("MarkSelfManaged stamp LOST for %d tool(s) — the claude-cli bridge drops them once promoted:\n  %s",
			len(lost), strings.Join(lost, "\n  "))
	}
	if len(added) > 0 {
		t.Errorf("MarkSelfManaged stamp ADDED for %d tool(s) not in the golden set:\n  %s",
			len(added), strings.Join(added, "\n  "))
	}
}

// TestTierParityTierValuesAreKnown guards the census itself: every value in the
// golden tables must be one of the tools package's real tier constants, so a typo
// in the literal cannot silently make an assertion vacuous.
func TestTierParityTierValuesAreKnown(t *testing.T) {
	known := map[string]bool{
		tools.VisibilityFull:     true,
		tools.VisibilitySummary:  true,
		tools.VisibilityNameOnly: true,
		tools.VisibilityHidden:   true,
	}
	for _, table := range []map[string]string{goldenTiersAuto, goldenTiersReadOnly} {
		for name, tier := range table {
			if !known[tier] {
				t.Errorf("golden entry %q has unknown tier %q", name, tier)
			}
		}
	}
}
