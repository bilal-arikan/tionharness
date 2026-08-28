package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// A tool blocked for THIS agent must not appear when the agent opens the bundle
// it belongs to: buildRegistry hands activate_tools an index built with the
// agent's own tool filter. Its unblocked sibling in the same bundle still shows.
func TestBundleIndexHonoursAgentToolFilter(t *testing.T) {
	ctx := context.Background()
	rt, ag := newOverrideRuntime(t, `{"get_flow":"blocked"}`)

	reg := rt.buildRegistry(ctx, ag)
	args, err := json.Marshal(map[string]any{
		"names": []string{tools.GroupPrefix + tools.CategoryAutomation},
	})
	if err != nil {
		t.Fatal(err)
	}
	res := reg.Call(ctx, providers.ToolCall{Name: "activate_tools", Input: args})
	if res.IsError {
		t.Fatalf("activate_tools failed: %s", res.Content)
	}
	if strings.Contains(res.Content, "- get_flow ") || strings.Contains(res.Content, "- get_flow\n") {
		t.Fatalf("blocked tool listed in its bundle:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "- list_flows") {
		t.Fatalf("unblocked sibling missing from the bundle listing:\n%s", res.Content)
	}
}

// The advertised bundle counts come from the same filtered index, so the number
// in the catalog block's "Bundles:" line never exceeds the listing an agent gets.
func TestLazyBundleCountsHonoursAgentToolFilter(t *testing.T) {
	ctx := context.Background()
	rt, ag := newOverrideRuntime(t, `{"get_flow":"blocked"}`)

	reg := rt.buildRegistry(ctx, ag)
	key := tools.GroupPrefix + tools.CategoryAutomation
	unfiltered := lazyBundleCounts(reg, nil)[key]
	filtered := lazyBundleCounts(reg, rt.toolFilter(ctx, ag))[key]
	if unfiltered == 0 {
		t.Fatalf("fixture broken: %s has no lazy members", key)
	}
	if filtered != unfiltered-1 {
		t.Fatalf("blocked tool still counted: filtered=%d unfiltered=%d", filtered, unfiltered)
	}
}
