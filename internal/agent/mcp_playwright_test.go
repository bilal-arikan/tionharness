package agent

import (
	"slices"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestIsFileWritingMCP(t *testing.T) {
	cases := []struct {
		name    string
		command string
		args    string
		want    bool
	}{
		{"playwright via bunx args", "bunx", `["@playwright/mcp","--browser","chrome"]`, true},
		{"playwright in command", "playwright-mcp", "[]", true},
		{"uppercase Playwright", "npx", `["@Playwright/MCP"]`, true},
		{"unrelated server", "npx", `["@modelcontextprotocol/server-filesystem"]`, false},
		{"codebase memory", "codebase-memory-mcp.exe", "[]", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isFileWritingMCP(db.MCPServer{Command: c.command, Args: c.args})
			if got != c.want {
				t.Fatalf("isFileWritingMCP(%q,%q) = %v, want %v", c.command, c.args, got, c.want)
			}
		})
	}
}

func TestEnsureOutputDirArg(t *testing.T) {
	pad := `C:\store\sessions\SES4\scratchpad`

	// Injected when absent.
	got := ensureOutputDirArg([]string{"--browser", "chrome"}, pad)
	want := []string{"--browser", "chrome", "--output-dir=" + pad}
	if !slices.Equal(got, want) {
		t.Fatalf("inject: got %v, want %v", got, want)
	}

	// Respected (not duplicated) when the operator set --output-dir=... form.
	in := []string{"--output-dir=" + `D:\custom`}
	if got := ensureOutputDirArg(in, pad); !slices.Equal(got, in) {
		t.Fatalf("respect =form: got %v, want %v", got, in)
	}

	// Respected when the operator set the two-token --output-dir X form.
	in2 := []string{"--output-dir", `D:\custom`}
	if got := ensureOutputDirArg(in2, pad); !slices.Equal(got, in2) {
		t.Fatalf("respect two-token: got %v, want %v", got, in2)
	}

	// Input slice is never mutated.
	orig := []string{"--browser", "chrome"}
	_ = ensureOutputDirArg(orig, pad)
	if len(orig) != 2 {
		t.Fatalf("input mutated: %v", orig)
	}
}
