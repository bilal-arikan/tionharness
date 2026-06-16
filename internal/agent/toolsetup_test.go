package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// newTestRuntime builds a Runtime backed by a real file store and a workspace
// sandbox rooted at workDir — mirroring how the workspace manager wires it.
func newTestRuntime(t *testing.T, workDir string) (*Runtime, *Tunables) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	tun := NewTunables()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := NewRuntime(database, providers.NewRegistry(""), tun, workDir, nil, "", "", logger)
	return rt, tun
}

// callTool dispatches a tool call through the exact path the native agentic loop
// uses (tools.Registry.Call), returning the result content.
func callTool(t *testing.T, rt *Runtime, agent db.Agent, name string, args map[string]any) providers.ToolResult {
	t.Helper()
	input, _ := json.Marshal(args)
	reg := rt.buildRegistry(context.Background(), agent)
	if !reg.Has(name) {
		t.Fatalf("tool %q not registered", name)
	}
	return reg.Call(context.Background(), providers.ToolCall{ID: "call_1", Name: name, Input: input})
}

// TestBuiltinFSToolsThroughRegistry exercises the production dispatch path:
// Runtime.buildRegistry -> Registry.Call -> sandbox -> real filesystem.
func TestBuiltinFSToolsThroughRegistry(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "FS", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// write_file then read_file round-trips real bytes on disk.
	if res := callTool(t, rt, agent, "write_file", map[string]any{
		"path": "docs/note.txt", "content": "hello sandbox",
	}); res.IsError {
		t.Fatalf("write_file errored: %s", res.Content)
	}
	if _, err := os.Stat(filepath.Join(workDir, "docs", "note.txt")); err != nil {
		t.Fatalf("file not written to sandbox: %v", err)
	}
	if res := callTool(t, rt, agent, "read_file", map[string]any{"path": "docs/note.txt"}); res.IsError || res.Content != "hello sandbox" {
		t.Fatalf("read_file got %q (err=%v)", res.Content, res.IsError)
	}

	// edit_file mutates the file.
	if res := callTool(t, rt, agent, "edit_file", map[string]any{
		"path": "docs/note.txt", "old_string": "hello", "new_string": "HELLO",
	}); res.IsError {
		t.Fatalf("edit_file errored: %s", res.Content)
	}
	if res := callTool(t, rt, agent, "read_file", map[string]any{"path": "docs/note.txt"}); res.Content != "HELLO sandbox" {
		t.Fatalf("after edit got %q", res.Content)
	}

	// list_dir, glob and grep see the new file.
	if res := callTool(t, rt, agent, "list_dir", map[string]any{"path": "docs"}); !strings.Contains(res.Content, "note.txt") {
		t.Fatalf("list_dir got %q", res.Content)
	}
	if res := callTool(t, rt, agent, "glob", map[string]any{"pattern": "**/*.txt"}); !strings.Contains(res.Content, "docs/note.txt") {
		t.Fatalf("glob got %q", res.Content)
	}
	if res := callTool(t, rt, agent, "grep", map[string]any{"pattern": "HELLO"}); !strings.Contains(res.Content, "docs/note.txt:1:HELLO sandbox") {
		t.Fatalf("grep got %q", res.Content)
	}

	// A path-traversal attempt is rejected as an error result, not executed.
	if res := callTool(t, rt, agent, "read_file", map[string]any{"path": "../escape.txt"}); !res.IsError {
		t.Fatalf("expected sandbox escape to be rejected, got %q", res.Content)
	}
}

// TestShellToolGate verifies the shell tool is absent until enabled, then runs
// a real command inside the sandbox once turned on.
func TestShellToolGate(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, tun := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Sh", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Off by default.
	if rt.buildRegistry(ctx, agent).Has("shell") {
		t.Fatal("shell tool must be absent when disabled")
	}

	// Enabled → present and actually executes.
	tun.SetShellEnabled(true)
	res := callTool(t, rt, agent, "shell", map[string]any{"command": "echo swarmgo-shell-ok"})
	if res.IsError || !strings.Contains(res.Content, "swarmgo-shell-ok") {
		t.Fatalf("shell run got %q (err=%v)", res.Content, res.IsError)
	}
}
