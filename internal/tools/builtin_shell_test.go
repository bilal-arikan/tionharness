package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUnwrapRedundantPowershell checks that a redundant outer
// `powershell -Command "..."` wrapper is stripped (so $variables survive), while
// non-wrapper commands and ambiguous escaped-quote cases are left untouched.
func TestUnwrapRedundantPowershell(t *testing.T) {
	cases := []struct{ in, want string }{
		// Redundant wrapper → unwrapped.
		{`powershell -Command "$x = 1; $x"`, `$x = 1; $x`},
		{`powershell.exe -NoProfile -Command "Get-Date"`, `Get-Date`},
		{`powershell -c "Write-Host hi"`, `Write-Host hi`},
		{`pwsh -Command "Get-Date"`, `Get-Date`},
		// Not a wrapper → unchanged.
		{`$x = Invoke-RestMethod $u; $x.city`, `$x = Invoke-RestMethod $u; $x.city`},
		{`Get-ChildItem`, `Get-ChildItem`},
		// Escaped quotes inside → too risky to unwrap, left as-is.
		{`powershell -Command "Write-Host \"hi\""`, `powershell -Command "Write-Host \"hi\""`},
	}
	for _, c := range cases {
		if got := unwrapRedundantPowershell(c.in); got != c.want {
			t.Errorf("unwrapRedundantPowershell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestShellToolsAvailability verifies at least one shell is always available on
// the current OS (Unix→Bash via /bin/sh, Windows→PowerShell via powershell.exe),
// so a ShellEnabled workspace never ends up with no shell at all.
func TestShellToolsAvailability(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	bash := NewShellTool(sb).Available()
	ps := NewPowerShellTool(sb).Available()
	if !bash && !ps {
		t.Fatal("expected at least one shell (Bash or PowerShell) to be available")
	}
	if runtime.GOOS != "windows" && !bash {
		t.Error("Bash must be available on Unix (/bin/sh)")
	}
	if runtime.GOOS == "windows" && !ps {
		t.Error("PowerShell must be available on Windows (powershell.exe)")
	}
}

// TestShellCallStreamEmitsChunks verifies the Bash tool implements StreamingTool
// and forwards output to onChunk while still returning the full result.
func TestShellCallStreamEmitsChunks(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)
	if !tool.Available() {
		t.Skip("no POSIX shell available on this host")
	}

	// Sanity: it is recognised as a streaming tool by the registry.
	reg := NewRegistry(tool)
	if !reg.CanStream("Bash") {
		t.Fatal("shell should be a StreamingTool")
	}

	var chunks []string
	out, err := tool.CallStream(
		context.Background(),
		json.RawMessage(`{"command":"echo streamtest"}`),
		func(c string) { chunks = append(chunks, c) },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "streamtest") {
		t.Fatalf("full output missing token: %q", out)
	}
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "streamtest") {
		t.Fatalf("streamed chunks missing token: %q", joined)
	}
}

// TestShellOutputFilter verifies WithOutputFilter post-processes the RETURNED
// output above shellCompressMinBytes (the token-optimizer hook) but leaves small
// outputs untouched, and that the filter receives the command + the full raw output.
func TestShellOutputFilter(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	base := NewShellTool(sb)
	if !base.Available() {
		t.Skip("no POSIX shell available on this host")
	}

	// A recording filter that reports it ran and echoes back a sentinel-wrapped output.
	var gotCmd, gotOut string
	called := 0
	tool := base.WithOutputFilter(func(cmd, output string) string {
		called++
		gotCmd, gotOut = cmd, output
		return "FILTERED\n" + output
	})

	// Large output (> shellCompressMinBytes): write a file in the sandbox and cat it,
	// so the size is deterministic and portable (no seq/head dependency).
	big := strings.Repeat("abcdefghij\n", 300) // 3300 bytes > 2048
	if err := os.WriteFile(filepath.Join(sb.Root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}
	out, err := tool.Call(context.Background(), json.RawMessage(`{"command":"cat big.txt"}`))
	if err != nil {
		t.Fatalf("cat big.txt: %v", err)
	}
	if called != 1 {
		t.Fatalf("filter should run once for large output, ran %d times", called)
	}
	if !strings.HasPrefix(out, "FILTERED\n") {
		t.Fatalf("large output should be filtered, got prefix %q", out[:min(20, len(out))])
	}
	if gotCmd != "cat big.txt" {
		t.Fatalf("filter should receive the command, got %q", gotCmd)
	}
	if !strings.Contains(gotOut, "abcdefghij") || len(gotOut) < shellCompressMinBytes {
		t.Fatalf("filter should receive the full raw output (%d bytes)", len(gotOut))
	}

	// Small output (< shellCompressMinBytes): filter must NOT run (passthrough).
	called = 0
	out, err = tool.Call(context.Background(), json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("echo hi: %v", err)
	}
	if called != 0 {
		t.Fatal("filter must be skipped for small output")
	}
	if !strings.Contains(out, "hi") || strings.HasPrefix(out, "FILTERED") {
		t.Fatalf("small output should pass through unfiltered, got %q", out)
	}

	// no_compress:true opts out per call, even for large output.
	called = 0
	out, err = tool.Call(context.Background(), json.RawMessage(`{"command":"cat big.txt","no_compress":true}`))
	if err != nil {
		t.Fatalf("cat big.txt no_compress: %v", err)
	}
	if called != 0 {
		t.Fatal("no_compress must skip the filter")
	}
	if strings.HasPrefix(out, "FILTERED") || !strings.Contains(out, "abcdefghij") {
		t.Fatalf("no_compress output should be raw, got prefix %q", out[:min(20, len(out))])
	}
}

// TestPowerShellCallStream verifies the PowerShell tool runs a command and
// streams output. Skips where no PowerShell host is installed (e.g. bare Linux).
func TestPowerShellCallStream(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewPowerShellTool(sb)
	if !tool.Available() {
		t.Skip("no PowerShell host available on this host")
	}
	if !NewRegistry(tool).CanStream("PowerShell") {
		t.Fatal("PowerShell should be a StreamingTool")
	}

	out, err := tool.CallStream(
		context.Background(),
		json.RawMessage(`{"command":"Write-Output pstest"}`),
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "pstest") {
		t.Fatalf("PowerShell output missing token: %q", out)
	}
}
