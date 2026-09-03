package repair

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const notIndexedBody = `{"error":"project not found or not indexed","hint":"Use list_projects to see all indexed projects, then pass it as the \"project\" argument.","available_projects":["C-Users-user-Desktop-Projects-external-context-agent","C-Users-user-Desktop-Projects-TionHarness"],"count":2}`

// oneProjectBody is the shape the server returns when a single repo is indexed —
// the case where a missing `project` has exactly one defensible answer.
const oneProjectBody = `{"error":"project not found or not indexed","available_projects":["C-Users-user-Desktop-Projects-SampleRepo"],"count":1}`

// tionharnessCwd is a session working directory whose derived project id is
// present in notIndexedBody's available_projects.
const tionharnessCwd = `C:\Users\user\Desktop\Projects\TionHarness`

// searchTool is built through the SAME helper the registry uses, so a change to
// the namespace format breaks these tests instead of silently disabling the
// guard in production. The guard once matched only the claude-cli "mcp__" prefix
// while the native loop produced "<server>__<tool>", and every test passed
// because it hardcoded the CLI form.
var searchTool = mcp.NamespaceTool("codebase-memory-mcp", "search_code")

func mcpCall(tool string, args map[string]any) providers.ToolCall {
	raw, _ := json.Marshal(args)
	return providers.ToolCall{Name: tool, Input: raw}
}

func errResult(body string) providers.ToolResult {
	return providers.ToolResult{Content: body, IsError: true}
}

func TestMCPRepair_FiresOnNativeLoopToolName(t *testing.T) {
	if strings.HasPrefix(searchTool, "mcp__") {
		t.Fatalf("registry namespace unexpectedly carries the CLI prefix: %q", searchTool)
	}
	m := NewGuard()
	call := mcpCall(searchTool, map[string]any{"project": "nope", "pattern": "foo"})
	if _, ok := m.Repair(call, errResult(notIndexedBody), ""); !ok {
		t.Fatal("repair must fire on the native loop's <server>__<tool> name")
	}
}

func TestMCPRepair_AutoCorrectsMissingProjectFromSessionCwd(t *testing.T) {
	m := NewGuard()
	call := mcpCall(searchTool, map[string]any{"pattern": "queryToValues"})

	plan, ok := m.Repair(call, errResult(notIndexedBody), tionharnessCwd)
	if !ok {
		t.Fatal("expected repair to fire")
	}
	if plan.Fixed == nil {
		t.Fatalf("expected an auto-corrected call, got hint-only: %q", plan.Hint)
	}
	if got := CallProjectArg(*plan.Fixed); got != "C-Users-user-Desktop-Projects-TionHarness" {
		t.Errorf("project = %q, want the session repo's id", got)
	}
	// Every other argument survives the rewrite.
	var args map[string]any
	if err := json.Unmarshal(plan.Fixed.Input, &args); err != nil {
		t.Fatalf("rewritten input is not a JSON object: %v", err)
	}
	if args["pattern"] != "queryToValues" {
		t.Errorf("pattern lost in rewrite: %v", args)
	}
	// A corrected call must NOT be poisoned — the loop is about to run it.
	if blocked, _ := m.Precheck(*plan.Fixed); blocked {
		t.Error("the corrected call must not be pre-blocked")
	}
}

func TestMCPRepair_AutoCorrectsSingleIndexedProject(t *testing.T) {
	m := NewGuard()
	// No project argument, and the session cwd is unknown — one indexed repo is
	// still an unambiguous answer.
	plan, ok := m.Repair(mcpCall(searchTool, map[string]any{"pattern": "x"}), errResult(oneProjectBody), "")
	if !ok || plan.Fixed == nil {
		t.Fatalf("expected auto-correction to the only indexed project; plan=%+v", plan)
	}
	if got := CallProjectArg(*plan.Fixed); got != "C-Users-user-Desktop-Projects-SampleRepo" {
		t.Errorf("project = %q", got)
	}
}

func TestMCPRepair_AutoCorrectsBareRepoName(t *testing.T) {
	m := NewGuard()
	// The model named the repo instead of the project id.
	plan, ok := m.Repair(mcpCall(searchTool, map[string]any{"project": "TionHarness"}), errResult(notIndexedBody), "")
	if !ok || plan.Fixed == nil {
		t.Fatalf("expected a bare repo name to resolve; plan=%+v", plan)
	}
	if got := CallProjectArg(*plan.Fixed); got != "C-Users-user-Desktop-Projects-TionHarness" {
		t.Errorf("project = %q", got)
	}
}

func TestMCPRepair_AmbiguousArgumentIsNotGuessed(t *testing.T) {
	m := NewGuard()
	// "Projects" suffix-matches nothing, cwd is unknown, and two repos are indexed
	// → no single defensible answer, so the model must be told rather than sent to
	// an arbitrary repo.
	call := mcpCall(searchTool, map[string]any{"project": "some-other-repo"})
	plan, ok := m.Repair(call, errResult(notIndexedBody), "")
	if !ok {
		t.Fatal("expected repair to fire")
	}
	if plan.Fixed != nil {
		t.Fatalf("must not guess between two projects; got %q", CallProjectArg(*plan.Fixed))
	}
	if !strings.Contains(plan.Hint, "some-other-repo") {
		t.Errorf("hint should quote the rejected argument; got %q", plan.Hint)
	}
	if blocked, _ := m.Precheck(call); !blocked {
		t.Error("an uncorrectable call must be poisoned against an identical repeat")
	}
}

func TestMCPRepair_MissingArgumentHintDoesNotClaimUnindexed(t *testing.T) {
	m := NewGuard()
	// Missing project AND no way to derive it (unknown cwd, two candidates).
	plan, ok := m.Repair(mcpCall(searchTool, map[string]any{"pattern": "x"}), errResult(notIndexedBody), "")
	if !ok || plan.Fixed != nil {
		t.Fatalf("expected a hint-only plan; plan=%+v", plan)
	}
	if !strings.Contains(plan.Hint, "omitted the required `project`") {
		t.Errorf("hint must name the real fault (missing argument); got %q", plan.Hint)
	}
	// The old wording asserted the repo was unindexed and sent the agent to grep a
	// repo that was in fact indexed. That claim must not appear for this fault.
	if strings.Contains(plan.Hint, "does not match any indexed repo") {
		t.Errorf("hint must not misdiagnose a missing argument as an unindexed repo; got %q", plan.Hint)
	}
}

func TestMCPRepair_RewritesOnlyOncePerCall(t *testing.T) {
	m := NewGuard()
	call := mcpCall(searchTool, map[string]any{"pattern": "x"})

	plan, _ := m.Repair(call, errResult(notIndexedBody), tionharnessCwd)
	if plan.Fixed == nil {
		t.Fatal("setup: expected the first attempt to auto-correct")
	}
	// The corrected call failed too: the second pass must fall back to guidance
	// instead of rewriting again (which would ping-pong with the server).
	plan2, ok := m.Repair(call, errResult(notIndexedBody), tionharnessCwd)
	if !ok {
		t.Fatal("expected repair to fire on the retry failure")
	}
	if plan2.Fixed != nil {
		t.Error("a call may only be auto-corrected once per turn")
	}
	if plan2.Hint == "" {
		t.Error("the failed retry must still explain itself")
	}
}

func TestMCPRepair_StartsIndexWhenSessionRepoAbsent(t *testing.T) {
	m := NewGuard()
	// The session works on a repo the server has never indexed, and the argument
	// cannot be corrected to it.
	cwd := `C:\Users\user\Desktop\Projects\brand-new`
	plan, ok := m.Repair(mcpCall(searchTool, map[string]any{"project": "other"}), errResult(notIndexedBody), cwd)
	if !ok {
		t.Fatal("expected repair to fire")
	}
	if plan.IndexPath != cwd {
		t.Errorf("IndexPath = %q, want the session cwd %q", plan.IndexPath, cwd)
	}
	if !strings.Contains(plan.Hint, "background") {
		t.Errorf("hint should say an index was started; got %q", plan.Hint)
	}
}

func TestMCPRepair_NoIndexWhenSessionRepoAlreadyIndexed(t *testing.T) {
	m := NewGuard()
	// Two indexed repos, cwd is one of them, argument names neither → hint only.
	// Re-indexing an already-indexed repo here would be busywork.
	m.repaired[CallKey(mcpCall(searchTool, map[string]any{"project": "zzz"}))] = true
	plan, ok := m.Repair(mcpCall(searchTool, map[string]any{"project": "zzz"}), errResult(notIndexedBody), tionharnessCwd)
	if !ok {
		t.Fatal("expected repair to fire")
	}
	if plan.IndexPath != "" {
		t.Errorf("must not re-index a repo already in available_projects; got %q", plan.IndexPath)
	}
}

func TestMCPRepair_IgnoresSuccessAndNonMCP(t *testing.T) {
	m := NewGuard()

	// A successful MCP call must not poison anything.
	okCall := mcpCall(searchTool, map[string]any{"project": "good"})
	if _, ok := m.Repair(okCall, providers.ToolResult{Content: `{"results":[]}`, IsError: false}, ""); ok {
		t.Error("repair must not fire on a successful result")
	}
	if blocked, _ := m.Precheck(okCall); blocked {
		t.Error("a successful call must not be poisoned")
	}

	// A built-in tool that happens to carry the marker text is out of scope.
	biCall := mcpCall("Grep", map[string]any{"pattern": NotIndexedMarker})
	if _, ok := m.Repair(biCall, errResult(NotIndexedMarker), ""); ok {
		t.Error("repair must only act on namespaced MCP tools")
	}
	if blocked, _ := m.Precheck(biCall); blocked {
		t.Error("built-in tools must never be precheck-blocked")
	}
}

func TestMCPRepair_DifferentArgsNotBlocked(t *testing.T) {
	m := NewGuard()
	bad := mcpCall(searchTool, map[string]any{"project": "some-other-repo"})
	if _, ok := m.Repair(bad, errResult(notIndexedBody), ""); !ok {
		t.Fatal("setup: expected repair to fire")
	}
	// Same tool, corrected argument → must be allowed through (not the poisoned key).
	fixed := mcpCall(searchTool, map[string]any{"project": "C-Users-user-Desktop-Projects-TionHarness"})
	if blocked, _ := m.Precheck(fixed); blocked {
		t.Error("a call with corrected arguments must not be blocked")
	}
}

func TestMCPRepairInstruction_NamesSiblingTool(t *testing.T) {
	hint := mcpRepairInstruction(searchTool, "bad", []string{"C-Users-user-Desktop-Projects-TionHarness"})
	if !strings.Contains(hint, mcp.NamespaceTool("codebase-memory-mcp", "list_projects")) {
		t.Errorf("hint should name the list_projects sibling tool; got %q", hint)
	}
	if !strings.Contains(hint, "C-Users-user-Desktop-Projects-TionHarness") {
		t.Errorf("hint should list available projects; got %q", hint)
	}
	if !strings.Contains(hint, "Glob/Grep") {
		t.Errorf("hint should mention the Glob/Grep fallback; got %q", hint)
	}
}

func TestResolveProjectID(t *testing.T) {
	two := []string{"C-Users-user-Desktop-Projects-external-context-agent", "C-Users-user-Desktop-Projects-TionHarness"}
	cases := []struct {
		name      string
		want      string
		preferred string
		available []string
		expect    string
	}{
		{"no projects indexed", "", "C-Users-user-Desktop-Projects-TionHarness", nil, ""},
		{"already correct", "C-Users-user-Desktop-Projects-TionHarness", "", two, ""},
		{"missing, preferred indexed", "", "C-Users-user-Desktop-Projects-TionHarness", two, "C-Users-user-Desktop-Projects-TionHarness"},
		{"missing, preferred absent, two candidates", "", "C-Users-user-Desktop-Projects-other", two, ""},
		{"bare repo name", "external-context-agent", "", two, "C-Users-user-Desktop-Projects-external-context-agent"},
		{"case-insensitive full id", "c-users-user-desktop-projects-tionharness", "", two, "C-Users-user-Desktop-Projects-TionHarness"},
		{"absolute path", `C:\Users\user\Desktop\Projects\TionHarness`, "", two, "C-Users-user-Desktop-Projects-TionHarness"},
		{"unknown, falls back to preferred", "nope", "C-Users-user-Desktop-Projects-TionHarness", two, "C-Users-user-Desktop-Projects-TionHarness"},
		{"unknown, single index", "nope", "", []string{"C-Users-user-Desktop-Projects-SampleRepo"}, "C-Users-user-Desktop-Projects-SampleRepo"},
		{"unknown, no anchor", "nope", "", two, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveProjectID(tc.want, tc.preferred, tc.available); got != tc.expect {
				t.Errorf("resolveProjectID(%q, %q, %v) = %q, want %q", tc.want, tc.preferred, tc.available, got, tc.expect)
			}
		})
	}
}

func TestMatchProjectID_AmbiguityYieldsNothing(t *testing.T) {
	// Two indexed repos whose ids both end in "-api": no single answer.
	got := matchProjectID("api", []string{"C-Users-user-a-api", "C-Users-user-b-api"})
	if got != "" {
		t.Errorf("ambiguous suffix must not resolve; got %q", got)
	}
}

func TestWithProjectArg_RejectsNonObjectInput(t *testing.T) {
	call := providers.ToolCall{Name: searchTool, Input: json.RawMessage(`"not an object"`)}
	if _, err := withProjectArg(call, "x"); err == nil {
		t.Error("expected an error for non-object input rather than a silent rewrite")
	}
}

func TestParseAvailableProjects(t *testing.T) {
	got := parseAvailableProjects(notIndexedBody)
	want := []string{"C-Users-user-Desktop-Projects-external-context-agent", "C-Users-user-Desktop-Projects-TionHarness"}
	if len(got) != len(want) {
		t.Fatalf("got %d projects, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("project[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// A body with no JSON object yields nil, not a panic.
	if got := parseAvailableProjects("plain text error"); got != nil {
		t.Errorf("expected nil for non-JSON body, got %v", got)
	}
	// A not-indexed error without the list still parses to an empty/nil list
	// (instruction degrades gracefully to just "call list_projects").
	if got := parseAvailableProjects(`{"error":"project not found or not indexed"}`); len(got) != 0 {
		t.Errorf("expected no projects, got %v", got)
	}
}
