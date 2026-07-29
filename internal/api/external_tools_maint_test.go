package api

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestAnsiEscapeStripped locks the panel's readability. Both optimizers colourise
// their reports even when stdout is a pipe, and an unstripped escape renders as
// literal "[1m[36m" noise inside the report <pre>.
func TestAnsiEscapeStripped(t *testing.T) {
	// A real line from `sqz gain`, escapes and all.
	in := "\x1b[1m\x1b[36m📈 Token Savings\x1b[0m\x1b[0m\n  \x1b[2m07-28\x1b[0m │\x1b[92m███\x1b[0m│ \x1b[92m308391\x1b[0m saved"
	got := ansiEscapeRe.ReplaceAllString(in, "")
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("escape sequences survived: %q", got)
	}
	want := "📈 Token Savings\n  07-28 │███│ 308391 saved"
	if got != want {
		t.Fatalf("stripping altered the text\n got: %q\nwant: %q", got, want)
	}
	// Content that merely LOOKS like an escape (no ESC byte) must survive: a lint
	// report legitimately contains bracketed markers.
	plain := "[1m not an escape [FAIL] TestX"
	if out := ansiEscapeRe.ReplaceAllString(plain, ""); out != plain {
		t.Fatalf("plain text was mangled: %q", out)
	}
}

// TestRunToolCmd_MissingBinaryIsNotAnError: a tool that is not installed is a
// NORMAL state the panel renders as "not installed". Reporting it as an error
// would surface a red toast every time someone without rtk opens Settings.
func TestRunToolCmd_MissingBinaryIsNotAnError(t *testing.T) {
	out, found := runToolCmd(context.Background(), "tionswarm-no-such-binary-xyz")
	if found {
		t.Fatal("a nonexistent binary must report found=false")
	}
	if out != "" {
		t.Fatalf("expected no output for a missing binary, got %q", out)
	}
}

// TestRunToolCmd_IgnoresExitStatus guards the reason the helper drops the exit
// code: rtk exits non-zero on success paths (3 from `rtk rewrite`), so gating on
// status would discard perfectly good report text.
func TestRunToolCmd_IgnoresExitStatus(t *testing.T) {
	if _, err := exec.LookPath("cmd"); err != nil {
		t.Skip("cmd.exe not available on this host")
	}
	out, found := runToolCmd(context.Background(), "cmd", "/c", "echo REPORT & exit /b 3")
	if !found {
		t.Fatal("cmd resolved on PATH but was reported missing")
	}
	if !strings.Contains(out, "REPORT") {
		t.Fatalf("output must survive a non-zero exit, got %q", out)
	}
}

// TestRtkConfigPath keeps the reported location honest — the panel prints it and
// offers to open it, so a wrong path would send the user to a folder that has
// nothing to do with rtk.
func TestRtkConfigPath(t *testing.T) {
	p := rtkConfigPath()
	if p == "" {
		t.Skip("no config location resolvable in this environment")
	}
	if !strings.HasSuffix(strings.ToLower(p), "config.toml") {
		t.Errorf("path must point at config.toml, got %q", p)
	}
	if !strings.Contains(strings.ToLower(p), "rtk") {
		t.Errorf("path must be inside an rtk directory, got %q", p)
	}
}
