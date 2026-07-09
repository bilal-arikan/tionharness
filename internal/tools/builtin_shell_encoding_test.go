package tools

import "testing"

func TestIsWindowsPowerShell(t *testing.T) {
	cases := map[string]bool{
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`: true,
		"powershell.exe": true,
		"powershell":     true,
		"POWERSHELL.EXE": true,
		`C:\Program Files\PowerShell\7\pwsh.exe`: false,
		"pwsh.exe": false,
		"pwsh":     false,
		"/usr/bin/pwsh": false,
	}
	for exe, want := range cases {
		if got := isWindowsPowerShell(exe); got != want {
			t.Errorf("isWindowsPowerShell(%q) = %v, want %v", exe, got, want)
		}
	}
}

func TestApplyWinPSUTF8(t *testing.T) {
	// Legacy host: prelude is prepended, user command preserved verbatim after it.
	got := applyWinPSUTF8("powershell.exe", "Get-Content -Raw x.txt")
	if got == "Get-Content -Raw x.txt" {
		t.Fatal("expected prelude to be prepended for Windows PowerShell")
	}
	if !hasSuffix(got, "\nGet-Content -Raw x.txt") {
		t.Errorf("user command must follow the prelude verbatim, got: %q", got)
	}

	// pwsh 7+: already UTF-8, command must be returned unchanged (no prelude noise).
	if got := applyWinPSUTF8("pwsh.exe", "echo hi"); got != "echo hi" {
		t.Errorf("pwsh command must be unchanged, got: %q", got)
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
