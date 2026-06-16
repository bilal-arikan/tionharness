package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// callWithDiff dispatches a tool call through the production registry path with a
// diff sink attached (as the native tool loop does), returning the captured diff.
func callWithDiff(t *testing.T, rt *Runtime, agent db.Agent, name string, args map[string]any) (providers.ToolResult, *tools.FileDiff) {
	t.Helper()
	input, _ := json.Marshal(args)
	reg := rt.buildRegistry(context.Background(), agent)
	ctx, sink := tools.WithDiffSink(context.Background())
	res := reg.Call(ctx, providers.ToolCall{ID: "c1", Name: name, Input: input})
	return res, sink.Take()
}

// TestFSToolsEmitDiff verifies that write_file and edit_file surface a structured
// FileDiff through the diff sink — the data the chat renders as a diff card.
func TestFSToolsEmitDiff(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	agent, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Diff", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// New file → Created, all lines added.
	res, d := callWithDiff(t, rt, agent, "write_file", map[string]any{
		"path": "a.txt", "content": "one\ntwo\nthree\n",
	})
	if res.IsError {
		t.Fatalf("write errored: %s", res.Content)
	}
	if d == nil || !d.Created || d.Added != 3 || d.Removed != 0 || d.Path != "a.txt" {
		t.Fatalf("write diff = %+v, want created a.txt +3 -0", d)
	}

	// Edit one line → one added, one removed.
	res, d = callWithDiff(t, rt, agent, "edit_file", map[string]any{
		"path": "a.txt", "old_string": "two", "new_string": "TWO",
	})
	if res.IsError {
		t.Fatalf("edit errored: %s", res.Content)
	}
	if d == nil || d.Created || d.Added != 1 || d.Removed != 1 {
		t.Fatalf("edit diff = %+v, want +1 -1", d)
	}
	if d.Patch == "" {
		t.Error("expected a non-empty patch for a small edit")
	}
}
