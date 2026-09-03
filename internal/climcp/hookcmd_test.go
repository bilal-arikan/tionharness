package climcp

import (
	"runtime"
	"strings"
	"testing"
)

// A PowerShell-authored hook handed to the CLI verbatim dies under its POSIX shell
// with `syntax error near unexpected token '|'`, which the CLI then treats as a
// refusal — disabling the matched tool for the whole session. On Windows the
// command must be re-wrapped so the interpreter matches the dialect.
func TestCLIHookCommandWrapsPowerShellSource(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("wrapping only applies where execHook authors hooks in PowerShell (windows)")
	}
	// The exact rtk optimizer hook found live across five workspaces.
	src := `$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; ` +
		`if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c; ` +
		`@{updatedInput=$j.tool_input}|ConvertTo-Json -Compress }`

	got := HookCommand(src)
	if !strings.HasPrefix(got, "powershell.exe -NoProfile -NonInteractive -Command ") {
		t.Fatalf("PowerShell source was not wrapped for the CLI's POSIX shell\ngot: %s", got)
	}
	// The body must survive intact inside a single POSIX-quoted word: its embedded
	// single quotes are the reason a naive wrap breaks.
	body := strings.TrimPrefix(got, "powershell.exe -NoProfile -NonInteractive -Command ")
	if !strings.HasPrefix(body, "'") || !strings.HasSuffix(body, "'") {
		t.Fatalf("body is not single-quoted for sh\nbody: %s", body)
	}
	if !strings.Contains(body, `'\''rtk *'\''`) {
		t.Errorf("embedded quotes not escaped the POSIX way ('\\'')\nbody: %s", body)
	}
	// Unquoting the sh word must reproduce the original source byte-for-byte:
	// strip the outer quotes, then undo the '\'' escape.
	inner := strings.TrimSuffix(strings.TrimPrefix(body, "'"), "'")
	if unquoted := strings.ReplaceAll(inner, `'\''`, "'"); unquoted != src {
		t.Errorf("round-trip through sh quoting altered the command\nwant: %s\n got: %s", src, unquoted)
	}
}

// Commands that already name their interpreter are plain executable invocations
// the CLI's shell runs fine. Wrapping them would break their own quoting, so they
// must pass through byte-identical. All three forms are live workspace hooks.
func TestCLIHookCommandLeavesExecutableInvocationsAlone(t *testing.T) {
	for _, cmd := range []string{
		`sqz hook claude`,
		`powershell -NoProfile -ExecutionPolicy Bypass -File "C:\Users\user\Desktop\Progs\sqz\sqz-bridge-hook.ps1"`,
		`node /c/hooks/rewrite.js`,
		`rtk hook claude --json`,
		`/usr/bin/env python3 hook.py`,
	} {
		if got := HookCommand(cmd); got != cmd {
			t.Errorf("command was rewritten but should pass through\n in: %s\nout: %s", cmd, got)
		}
	}
}

// An empty command carries no dialect to translate.
func TestCLIHookCommandEmpty(t *testing.T) {
	if got := HookCommand(""); got != "" {
		t.Errorf("empty command became %q", got)
	}
}

// needsPowerShellWrap is the discriminator; assert both directions directly so a
// future edit cannot quietly start wrapping ordinary programs.
func TestNeedsPowerShellWrap(t *testing.T) {
	powershellSource := []string{
		`$j=[Console]::In.ReadToEnd()`,
		`[Console]::Error.WriteLine("x")`,
		`& 'C:\Program Files\tool\run.ps1'`,
		`. .\profile.ps1`,
		`Get-Content foo.json`,
		`ConvertFrom-Json`,
	}
	for _, c := range powershellSource {
		if !needsPowerShellWrap(c) {
			t.Errorf("PowerShell source not detected: %s", c)
		}
	}
	plainPrograms := []string{
		`sqz hook claude`,
		`rtk-wrapper.sh --json`, // hyphen, but has an extension
		`some/dir-name/run`,     // hyphen, but has a separator
		`node hook.js`,
		`powershell -File x.ps1`,
		`echo hi | grep hi`,
	}
	for _, c := range plainPrograms {
		if needsPowerShellWrap(c) {
			t.Errorf("plain program misdetected as PowerShell source: %s", c)
		}
	}
}
