package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDialStdioSurfacesServerStderr is the regression for the 2026-08-27 blind
// spot: a stdio MCP server that explains itself on stderr and exits must have that
// explanation attached to the dial error, not replaced by a bare "EOF".
func TestDialStdioSurfacesServerStderr(t *testing.T) {
	if os.Getenv("MCP_STDERR_HELPER") == "1" {
		fmt.Fprintln(os.Stderr, "level=info msg=banner noise nobody needs")
		fmt.Fprintln(os.Stderr, "CBM daemon is active or starting but could not accept this client within 30000 ms")
		os.Exit(1)
	}

	_, err := DialStdio(context.Background(), os.Args[0],
		[]string{"-test.run=TestDialStdioSurfacesServerStderr"},
		[]string{"MCP_STDERR_HELPER=1"}, "")
	if err == nil {
		t.Fatal("expected dial to fail when the server exits during initialize")
	}
	if !strings.Contains(err.Error(), "could not accept this client") {
		t.Fatalf("server stderr not surfaced in dial error: %v", err)
	}
}

// TestStderrTailIsBoundedAndNonBlocking guards the two properties that let us keep
// stderr at all: it never reports a short write (which would make os/exec stop
// draining the pipe and reintroduce the blocking this replaced) and it never grows
// past the cap no matter how chatty the server is.
func TestStderrTailIsBoundedAndNonBlocking(t *testing.T) {
	s := &stderrTail{}
	chunk := []byte(strings.Repeat("x", 1024))
	for i := 0; i < 100; i++ {
		n, err := s.Write(chunk)
		if err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if n != len(chunk) {
			t.Fatalf("short write reported at %d: %d of %d", i, n, len(chunk))
		}
	}
	if len(s.buf) > stderrTailCap {
		t.Fatalf("buffer grew past cap: %d > %d", len(s.buf), stderrTailCap)
	}

	// A single write larger than the cap keeps the END of it — that is where a
	// failing process puts its final, actionable message.
	big := append([]byte(strings.Repeat("y", stderrTailCap*2)), []byte("FINAL")...)
	if _, err := s.Write(big); err != nil {
		t.Fatalf("oversized write: %v", err)
	}
	if len(s.buf) > stderrTailCap {
		t.Fatalf("oversized write grew past cap: %d", len(s.buf))
	}
	if !strings.HasSuffix(s.Tail(), "FINAL") {
		t.Fatalf("oversized write kept the wrong end: %q", s.Tail())
	}
}

// TestStderrTailLastLinesKeepsActionableEnd confirms the banner/allocator noise a
// server prints before failing is dropped in favour of its final lines.
func TestStderrTailLastLinesKeepsActionableEnd(t *testing.T) {
	s := &stderrTail{}
	if _, err := s.Write([]byte("banner\n\nlevel=info msg=noise\nreal error here\n")); err != nil {
		t.Fatal(err)
	}
	got := s.lastLines(2)
	if got != "level=info msg=noise | real error here" {
		t.Fatalf("lastLines = %q", got)
	}
	if s.lastLines(1) != "real error here" {
		t.Fatalf("lastLines(1) = %q", s.lastLines(1))
	}
}

var _ = exec.Command // keep os/exec referenced if DialStdio's wrapper changes
