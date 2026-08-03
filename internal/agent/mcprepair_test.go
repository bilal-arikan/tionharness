package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

const notIndexedBody = `{"error":"project not found or not indexed","hint":"Use list_projects to see all indexed projects, then pass it as the \"project\" argument.","available_projects":["C-Users-user-Desktop-Projects-external-context-agent","C-Users-user-Desktop-Projects-TionSwarm"],"count":2}`

func mcpCall(tool string, args map[string]any) providers.ToolCall {
	raw, _ := json.Marshal(args)
	return providers.ToolCall{Name: tool, Input: raw}
}

func TestMCPRepair_AppendsInstructionAndPoisons(t *testing.T) {
	m := newMCPRepair()
	call := mcpCall("mcp__codebase-memory-mcp__search_code", map[string]any{"project": "bad", "pattern": "foo"})
	res := providers.ToolResult{Content: notIndexedBody, IsError: true}

	hint, ok := m.repair(call, res)
	if !ok {
		t.Fatal("expected repair to fire on a not-indexed error body")
	}
	// Names the exact sibling recovery tool on the same server.
	if !strings.Contains(hint, "mcp__codebase-memory-mcp__list_projects") {
		t.Errorf("hint should name the list_projects sibling tool; got %q", hint)
	}
	// Surfaces the available projects verbatim.
	if !strings.Contains(hint, "C-Users-user-Desktop-Projects-TionSwarm") {
		t.Errorf("hint should list available projects; got %q", hint)
	}
	// The Glob/Grep fallback is prescribed.
	if !strings.Contains(hint, "Glob/Grep") {
		t.Errorf("hint should mention the Glob/Grep fallback; got %q", hint)
	}
	// The identical call is now poisoned.
	blocked, msg := m.precheck(call)
	if !blocked {
		t.Fatal("expected the identical repeat to be blocked after a not-indexed error")
	}
	if !strings.Contains(msg, "list_projects") {
		t.Errorf("block message should direct to list_projects; got %q", msg)
	}
}

func TestMCPRepair_IgnoresSuccessAndNonMCP(t *testing.T) {
	m := newMCPRepair()

	// A successful MCP call must not poison anything.
	okCall := mcpCall("mcp__codebase-memory-mcp__search_code", map[string]any{"project": "good"})
	if _, ok := m.repair(okCall, providers.ToolResult{Content: `{"results":[]}`, IsError: false}); ok {
		t.Error("repair must not fire on a successful result")
	}
	if blocked, _ := m.precheck(okCall); blocked {
		t.Error("a successful call must not be poisoned")
	}

	// A non-MCP tool that happens to carry the marker text is out of scope.
	biCall := mcpCall("grep", map[string]any{"pattern": mcpNotIndexedMarker})
	if _, ok := m.repair(biCall, providers.ToolResult{Content: mcpNotIndexedMarker, IsError: true}); ok {
		t.Error("repair must only act on mcp__ tools")
	}
	if blocked, _ := m.precheck(biCall); blocked {
		t.Error("non-mcp tools must never be precheck-blocked")
	}
}

func TestMCPRepair_DifferentArgsNotBlocked(t *testing.T) {
	m := newMCPRepair()
	bad := mcpCall("mcp__codebase-memory-mcp__search_code", map[string]any{"project": "bad"})
	if _, ok := m.repair(bad, providers.ToolResult{Content: notIndexedBody, IsError: true}); !ok {
		t.Fatal("setup: expected repair to fire")
	}
	// Same tool, corrected argument → must be allowed through (not the poisoned key).
	fixed := mcpCall("mcp__codebase-memory-mcp__search_code", map[string]any{"project": "C-Users-user-Desktop-Projects-TionSwarm"})
	if blocked, _ := m.precheck(fixed); blocked {
		t.Error("a call with corrected arguments must not be blocked")
	}
}

func TestParseAvailableProjects(t *testing.T) {
	got := parseAvailableProjects(notIndexedBody)
	want := []string{"C-Users-user-Desktop-Projects-external-context-agent", "C-Users-user-Desktop-Projects-TionSwarm"}
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
