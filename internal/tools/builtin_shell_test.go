package tools

import (
	"context"
	"encoding/json"
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

// TestShellCallStreamEmitsChunks verifies the shell tool implements StreamingTool
// and forwards output to onChunk while still returning the full result.
func TestShellCallStreamEmitsChunks(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)

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
