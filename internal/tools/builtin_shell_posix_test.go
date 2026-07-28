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

func TestIsWSLBashLauncher(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path shape")
	}
	cases := map[string]bool{
		`C:\Windows\System32\bash.exe`:          true,
		`C:\Windows\system32\BASH.EXE`:          true,
		`C:\Windows\SysWOW64\bash.exe`:          true,
		`C:\Program Files\Git\bin\bash.exe`:     false,
		`C:\Program Files\Git\usr\bin\bash.exe`: false,
		`C:\Windows\System32\wsl.exe`:           false,
	}
	for p, want := range cases {
		if got := isWSLBashLauncher(p); got != want {
			t.Errorf("isWSLBashLauncher(%q) = %v, want %v", p, got, want)
		}
	}
}

// TestResolvePOSIXShellSkipsWSLLauncher plants a fake System32\bash.exe as the
// only bash on PATH and asserts the resolver refuses to back the Bash tool with
// it. Regression guard for the double-expansion bug documented on
// isWSLBashLauncher, which silently emptied every $VAR the script assigned.
func TestResolvePOSIXShellSkipsWSLLauncher(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolver branch")
	}
	fakeDir := filepath.Join(t.TempDir(), "System32")
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(fakeDir, "bash.exe")
	if err := os.WriteFile(fake, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir)

	exe, _, ok := resolvePOSIXShell()
	if ok && strings.EqualFold(exe, fake) {
		t.Fatalf("resolvePOSIXShell picked the WSL launcher %q", exe)
	}
}

// TestPOSIXShellExpandsVariables runs a real command through whatever shell the
// resolver chose and asserts that assignments and command substitution survive.
// This is the behavioural half of the regression: the WSL launcher returned
// "x=[] y=[]" here because an outer shell expanded the script first.
func TestPOSIXShellExpandsVariables(t *testing.T) {
	tool := NewShellTool(NewSandbox(t.TempDir()))
	if !tool.Available() {
		t.Skip("no POSIX shell on this host")
	}
	in, err := json.Marshal(shellArgs{
		Command:    `X=alpha; Y=$(printf beta); echo "x=[$X] y=[$Y]"`,
		TimeoutSec: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Call(context.Background(), in)
	if err != nil {
		t.Fatalf("shell call failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "x=[alpha] y=[beta]") {
		t.Fatalf("shell expansion broken via %q: got %q", tool.exe, out)
	}
}
