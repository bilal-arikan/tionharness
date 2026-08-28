package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// callMetaTool runs a load-on-demand meta-tool for an agent and fails on error.
func callMetaTool(t *testing.T, rt *Runtime, ag db.Agent, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, rt, ag, name, args)
	if res.IsError {
		t.Fatalf("%s failed: %s", name, res.Content)
	}
	return res.Content
}

// A tool blocked for THIS agent must be absent from the lazy catalog the
// load-on-demand meta-tools are built over: the prompt's catalog block is
// rendered filtered, so an unfiltered known set let activate_tools / tool_search
// resurrect a tool the agent may not use. Its unblocked sibling still resolves.
func TestLazyCatalogHonoursAgentToolFilter(t *testing.T) {
	rt, ag := newOverrideRuntime(t, `{"get_flow":"blocked"}`)

	out := callMetaTool(t, rt, ag, "activate_tools", map[string]any{"names": []string{"get_flow"}})
	if !strings.Contains(out, "Unknown names") || !strings.Contains(out, "get_flow") {
		t.Fatalf("blocked tool still activatable:\n%s", out)
	}

	out = callMetaTool(t, rt, ag, "activate_tools", map[string]any{"names": []string{"list_flows"}})
	if strings.Contains(out, "Unknown names") {
		t.Fatalf("unblocked sibling should activate:\n%s", out)
	}

	out = callMetaTool(t, rt, ag, "tool_search", map[string]any{"query": "get_flow list_flows"})
	if strings.Contains(out, "get_flow") {
		t.Fatalf("blocked tool surfaced by tool_search:\n%s", out)
	}
	if !strings.Contains(out, "list_flows") {
		t.Fatalf("unblocked sibling missing from tool_search:\n%s", out)
	}
}

// The eager name set handed to activate_tools is filtered too, so a blocked
// always-on tool reports as unknown instead of "already available (always-on)"
// — which would tell the agent to call a tool it does not have.
func TestEagerNamesHonourAgentToolFilter(t *testing.T) {
	ctx := context.Background()
	const eagerTool = "todo_write"

	base := newTestRegistryEagerNames(t, ctx)
	if !base[eagerTool] {
		t.Fatalf("fixture broken: %s is not eager", eagerTool)
	}

	rt, ag := newOverrideRuntime(t, `{"`+eagerTool+`":"blocked"}`)
	reg := rt.buildRegistry(ctx, ag)
	if names := reg.EagerNames(rt.toolFilter(ctx, ag)); names[eagerTool] {
		t.Fatalf("blocked tool still in the eager name set")
	}

	out := callMetaTool(t, rt, ag, "activate_tools", map[string]any{"names": []string{eagerTool}})
	if strings.Contains(out, "already available") {
		t.Fatalf("blocked eager tool advertised as available:\n%s", out)
	}
	if !strings.Contains(out, "Unknown names") {
		t.Fatalf("blocked eager tool should read as unknown:\n%s", out)
	}
}

// newTestRegistryEagerNames returns the unfiltered eager set of a bare agent,
// used as the baseline the filtered set is compared against.
func newTestRegistryEagerNames(t *testing.T, ctx context.Context) map[string]bool {
	t.Helper()
	rt, ag := newOverrideRuntime(t, `{}`)
	return rt.buildRegistry(ctx, ag).EagerNames(nil)
}
