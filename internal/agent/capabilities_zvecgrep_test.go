package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// zvecGrepRow is the server row the one-click wiring creates on a Windows nvm
// install: the npm shim, bridged to the shared daemon over stdio.
func zvecGrepRow() db.MCPServer {
	return db.MCPServer{
		Name: "zvec_grep", Transport: db.MCPTransportStdio, Enabled: true,
		Command: `C:\Users\user\.nvm\versions\node\v24.18.1\bin\zg.cmd`,
		Args:    `["server","--stdio"]`,
	}
}

func TestIsZvecGrepServer(t *testing.T) {
	stdio := func(command, args string) db.MCPServer {
		return db.MCPServer{Transport: db.MCPTransportStdio, Command: command, Args: args}
	}
	cases := []struct {
		name string
		row  db.MCPServer
		want bool
	}{
		{"windows shim", zvecGrepRow(), true},
		{"bare shim", stdio("zg", `["server","--stdio"]`), true},
		{"unix shim", stdio("/usr/local/bin/zg", ""), true},
		{"upper-case shim, legacy empty transport", db.MCPServer{Command: `C:\npm\ZG.CMD`}, true},
		{"node entry point", stdio("node", `["C:/npm/node_modules/@zvec/zvec-grep/dist/cli/index.js","server","--stdio"]`), true},
		{"npx package", stdio("npx", `["-y","@zvec/zvec-grep","server","--stdio"]`), true},
		{"zgrep is not zg", stdio("/usr/bin/zgrep", ""), false},
		{"zg-prefixed tool", stdio(`C:\tools\zg-helper.exe`, ""), false},
		{"http transport", db.MCPServer{Transport: db.MCPTransportHTTP, URL: "http://127.0.0.1:7999/mcp", Command: "zg"}, false},
		{"malformed args", stdio("node", "not json"), false},
		{"unrelated server", stdio("npx", `["@playwright/mcp"]`), false},
	}
	for _, c := range cases {
		if got := isZvecGrepServer(c.row); got != c.want {
			t.Errorf("%s: isZvecGrepServer = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestZvecGrepGuidance(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "repo")
	g := zvecGrepGuidance("zvec_grep", mcp.ServerAlive, "", cwd)
	for _, want := range []string{
		"zvec_grep__zvec_grep_search",
		"A zvec-grep MCP server is connected.",
		"`root`",
		"[INDEX_MISSING]",
		"zg index",
		"Glob/Grep",
		"`" + cwd + "`",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance missing %q\n%s", want, g)
		}
	}
	// A bare tool name is what made models guess a namespace (codebaseMemoryGuidance).
	if strings.Contains(g, " zvec_grep_search ") {
		t.Errorf("guidance exposes the bare tool name\n%s", g)
	}
	if strings.Contains(g, "search_graph") {
		t.Errorf("code-graph split rendered without a codebase-memory server\n%s", g)
	}

	// Unverified connection, codebase-memory on as well, no cwd.
	u := zvecGrepGuidance("zvec_grep", mcp.ServerUnknown, "codebase-memory-mcp", "")
	if strings.Contains(u, "is connected.") {
		t.Errorf("unverified state must not assert the connection\n%s", u)
	}
	for _, want := range []string{"not verified", "codebase-memory-mcp__search_graph", "codebase-memory-mcp__trace_path"} {
		if !strings.Contains(u, want) {
			t.Errorf("guidance missing %q\n%s", want, u)
		}
	}
	if strings.Contains(u, "working directory") {
		t.Errorf("cwd line rendered without a cwd\n%s", u)
	}

	// A throwaway worktree is never indexed, so the block must not promise an index.
	wt := filepath.Join(t.TempDir(), ".tionharness-worktrees", "WS1", "tsk1")
	if e := zvecGrepGuidance("zvec_grep", mcp.ServerAlive, "", wt); !strings.Contains(e, "never indexed") || strings.Contains(e, "built in the background") {
		t.Errorf("worktree cwd line is wrong\n%s", e)
	}

	if got := zvecGrepGuidance("", mcp.ServerAlive, "", cwd); got != "" {
		t.Errorf("expected no block without a server row, got %q", got)
	}
}

func TestZvecGrepCapabilityFollowsSwitchAndDenylist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateMCPServer(ctx, zvecGrepRow()); err != nil {
		t.Fatalf("create zvec-grep server: %v", err)
	}
	ag := db.Agent{ID: "AGT1", MCPEnabled: true}
	if !zvecGrepCapability.Detect(ctx, rt, ag) {
		t.Fatal("capability not detected for an enabled zvec-grep server")
	}
	if blk := rt.CapabilityContext(ctx, ag, ""); !strings.Contains(blk, "zvec_grep__zvec_grep_search") {
		t.Errorf("capability context lacks the zvec-grep block\n%s", blk)
	}

	// An explicit denylist removes the tool, so the block must go quiet.
	denied := db.Agent{ID: "AGT2", MCPEnabled: true, ToolOverrides: `{"zvec_grep__*":"blocked"}`}
	if zvecGrepCapability.Detect(ctx, rt, denied) {
		t.Error("block advertised to an agent whose denylist removes the tool")
	}

	rt.SetZvecGrep(false)
	if zvecGrepCapability.Detect(ctx, rt, ag) {
		t.Error("capability survived the workspace switch")
	}
	if blk := rt.CapabilityContext(ctx, ag, ""); strings.Contains(blk, "zvec_grep") {
		t.Errorf("switched-off workspace still carries the block\n%s", blk)
	}
}
