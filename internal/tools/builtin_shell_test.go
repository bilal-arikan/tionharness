package tools

import (
	"context"
	"encoding/json"
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
