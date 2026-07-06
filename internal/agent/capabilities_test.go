package agent

import (
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func TestProjectIDForPath(t *testing.T) {
	cases := map[string]string{
		"":                                      "",
		"   ":                                    "",
		`C:/Users/user/Desktop/Projects/TionSwarm`:                        "C-Users-user-Desktop-Projects-TionSwarm",
		`C:\Users\user\Desktop\Projects\TionSwarm`:                        "C-Users-user-Desktop-Projects-TionSwarm",
		`C:\Users\user\AppData\Local\Programs\@external-agentelectron`:       "C-Users-user-AppData-Local-Programs-external-agentelectron",
		`/home/user/my-repo`:                                               "home-user-my-repo",
	}
	for in, want := range cases {
		if got := projectIDForPath(in); got != want {
			t.Errorf("projectIDForPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodebaseMemoryCommand(t *testing.T) {
	stdio := db.MCPServer{Name: "cbm", Transport: db.MCPTransportStdio, Command: `C:\Progs\codebase-memory-mcp\codebase-memory-mcp.exe`}
	http := db.MCPServer{Name: "web", Transport: db.MCPTransportHTTP, URL: "http://x", Command: "codebase-memory-mcp"}
	other := db.MCPServer{Name: "brave", Transport: db.MCPTransportStdio, Command: "npx"}

	if got := codebaseMemoryCommand([]db.MCPServer{other, stdio}); got != stdio.Command {
		t.Errorf("expected stdio codebase-memory command, got %q", got)
	}
	if got := codebaseMemoryCommand([]db.MCPServer{other}); got != "" {
		t.Errorf("expected empty when absent, got %q", got)
	}
	// An http server that merely mentions the marker in Command must not match (the
	// auto-index helper shells out to a stdio executable only).
	if got := codebaseMemoryCommand([]db.MCPServer{http}); got != "" {
		t.Errorf("expected http transport to be skipped, got %q", got)
	}
}

func TestCBMStoreDir(t *testing.T) {
	if (&Runtime{}).CBMStoreDir() != "" {
		t.Error("expected empty store dir when workDir unknown")
	}
	container := t.TempDir()
	r := &Runtime{workDir: filepath.Join(container, "workspace")}
	want := filepath.Join(container, "cbm-store")
	if got := r.CBMStoreDir(); got != want {
		t.Errorf("CBMStoreDir() = %q, want %q", got, want)
	}
}
