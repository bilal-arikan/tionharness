package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildSourceRequiresExactlyOneSource: naming no source leaves nothing to
// watch, and naming several would silently watch the wrong one. Both are errors.
func TestBuildSourceRequiresExactlyOneSource(t *testing.T) {
	tool := NewMonitorTool(NewMonitorManager(nil), NewShellManager(), NewSandbox(t.TempDir()))

	if _, err := tool.buildSource(context.Background(), monitorSourceArgs{}); err == nil {
		t.Fatal("a start with no source was accepted")
	}
	_, err := tool.buildSource(context.Background(), monitorSourceArgs{ShellID: "bg1", Path: "a.log"})
	if err == nil {
		t.Fatal("a start naming two sources was accepted")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error %q does not explain the ambiguity", err)
	}
}

// TestBuildSourceRoutesByScheme: one `url` field must reach the polling source
// for http(s) and the socket source for ws(s), so the agent picks no backend.
func TestBuildSourceRoutesByScheme(t *testing.T) {
	tool := NewMonitorTool(NewMonitorManager(nil), NewShellManager(), NewSandbox(t.TempDir()))

	src, err := tool.buildSource(context.Background(), monitorSourceArgs{URL: "https://example.com/health"})
	if err != nil {
		t.Fatalf("https url: %v", err)
	}
	defer src.Close()
	if _, ok := src.(*urlSource); !ok {
		t.Fatalf("an https url built %T, not a polling source", src)
	}

	// A ws:// url must at least ROUTE to the socket source; the dial itself fails
	// here (nothing is listening), which is the correct arm-time error.
	if _, err = tool.buildSource(context.Background(), monitorSourceArgs{URL: "ws://example.invalid:1/x"}); err == nil {
		t.Fatal("a ws url to a dead host was accepted")
	}

	if _, err = tool.buildSource(context.Background(), monitorSourceArgs{URL: "ftp://example.com"}); err == nil {
		t.Fatal("a non-http/ws scheme was accepted")
	}
}

// TestBuildSourceFileUsesTheToolSandbox: a watched path must resolve against the
// tool's sandbox, not the process working directory.
func TestBuildSourceFileUsesTheToolSandbox(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tool := NewMonitorTool(NewMonitorManager(nil), NewShellManager(), NewSandbox(dir))

	src, err := tool.buildSource(context.Background(), monitorSourceArgs{Path: "app.log"})
	if err != nil {
		t.Fatalf("relative path did not resolve against the sandbox root: %v", err)
	}
	defer src.Close()
	if _, ok := src.(*fileSource); !ok {
		t.Fatalf("a path built %T, not a file source", src)
	}
}
