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

// TestResolvePOSIXShellPrefersGitBashOverWSL guards the SES49 regression: on a
// host where the only bash on PATH is the System32 WSL launcher but a real
// git-bash is installed, the resolver must fall through to git-bash rather than
// stop at the launcher or drop to wsl.exe. Under WSL the drive is mounted at
// /mnt/c, so a Windows-style "cd C:/..." fails ("No such file or directory") —
// exactly what broke that session. git-bash reaches C: as both /c/ and C:/, so
// the agent's Windows paths keep working only when git-bash wins.
func TestResolvePOSIXShellPrefersGitBashOverWSL(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolver branch")
	}
	const (
		launcher = `C:\Windows\System32\bash.exe`          // WSL launcher (must be rejected)
		gitBash  = `C:\Program Files\Git\usr\bin\bash.exe` // real git-bash (must win)
		wslExe   = `C:\Windows\System32\wsl.exe`           // last-resort fallback
	)
	restoreInterp, restoreGit := lookInterpreter, lookGitBash
	defer func() { lookInterpreter, lookGitBash = restoreInterp, restoreGit }()

	lookInterpreter = func(cands ...string) (string, bool) {
		switch cands[0] {
		case "bash":
			return launcher, true
		case "wsl":
			return wslExe, true
		}
		return "", false
	}
	lookGitBash = func() (string, bool) { return gitBash, true }

	exe, preArgs, ok := resolvePOSIXShell()
	if !ok {
		t.Fatal("resolvePOSIXShell returned ok=false with git-bash available")
	}
	if !strings.EqualFold(exe, gitBash) {
		t.Fatalf("resolver picked %q, want git-bash %q", exe, gitBash)
	}
	if len(preArgs) != 0 {
		t.Fatalf("git-bash needs no preArgs, got %v", preArgs)
	}
	if got := POSIXShellFlavor(); got != POSIXShellGitBash {
		t.Fatalf("POSIXShellFlavor() = %q, want %q (drives at /c/, C:/ works)", got, POSIXShellGitBash)
	}
}

// TestResolvePOSIXShellFallsBackToWSL documents the SES49 failure state itself:
// with no real bash and no git-bash, the resolver routes through "wsl.exe -e
// bash" and reports the wsl flavour so callers can warn the agent that drives
// live at /mnt/c and Windows-style C:/ paths will not resolve.
func TestResolvePOSIXShellFallsBackToWSL(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolver branch")
	}
	const (
		launcher = `C:\Windows\System32\bash.exe`
		wslExe   = `C:\Windows\System32\wsl.exe`
	)
	restoreInterp, restoreGit := lookInterpreter, lookGitBash
	defer func() { lookInterpreter, lookGitBash = restoreInterp, restoreGit }()

	lookInterpreter = func(cands ...string) (string, bool) {
		switch cands[0] {
		case "bash":
			return launcher, true // only the launcher — rejected
		case "wsl":
			return wslExe, true
		}
		return "", false
	}
	lookGitBash = func() (string, bool) { return "", false } // no git-bash on host

	exe, preArgs, ok := resolvePOSIXShell()
	if !ok {
		t.Fatal("resolvePOSIXShell returned ok=false with wsl available")
	}
	if !strings.EqualFold(exe, wslExe) {
		t.Fatalf("resolver picked %q, want wsl.exe %q", exe, wslExe)
	}
	if len(preArgs) < 2 || preArgs[0] != "-e" || preArgs[1] != "bash" {
		t.Fatalf("wsl route must run through \"-e bash\", got %v", preArgs)
	}
	if got := POSIXShellFlavor(); got != POSIXShellWSL {
		t.Fatalf("POSIXShellFlavor() = %q, want %q", got, POSIXShellWSL)
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
