package tools

import (
	"path/filepath"
	"strings"
)

// winPSUTF8Prelude makes Windows PowerShell 5.1 emit and read UTF-8 so non-ASCII
// content (e.g. Turkish text in settings files or agent souls) survives the
// round-trip to the Go parent process. Windows PowerShell 5.1 defaults its console
// output AND its file-reading cmdlets (Get-Content, Select-String, Import-Csv) to
// the legacy ANSI/OEM code page: a UTF-8-without-BOM file read on a Turkish system
// is decoded as cp1254 and the bytes reach Go as invalid UTF-8 (mojibake such as
// "talimatlar��"). PowerShell 7+ (pwsh) already defaults to UTF-8, so the
// prelude is applied ONLY to the legacy host.
//
// It touches only the OUTPUT encoding and the READ-side default of a few cmdlets —
// it deliberately does NOT set a global '*:Encoding' default, so file WRITES keep
// their existing (BOM-free) behavior and no silent regression is introduced for
// code that later reads those files.
const winPSUTF8Prelude = "$OutputEncoding=[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding $false;" +
	"$PSDefaultParameterValues['Get-Content:Encoding']='utf8';" +
	"$PSDefaultParameterValues['Select-String:Encoding']='utf8';" +
	"$PSDefaultParameterValues['Import-Csv:Encoding']='utf8';"

// isWindowsPowerShell reports whether exe is the legacy Windows PowerShell host
// (powershell.exe) rather than PowerShell 7+ (pwsh). Only the legacy host needs the
// UTF-8 prelude; pwsh is UTF-8 by default and non-Windows only ever resolves pwsh.
func isWindowsPowerShell(exe string) bool {
	base := strings.ToLower(filepath.Base(exe))
	base = strings.TrimSuffix(base, ".exe")
	return base == "powershell"
}

// applyWinPSUTF8 prepends winPSUTF8Prelude when the resolved host is Windows
// PowerShell 5.1. The prelude is a self-contained statement block terminated by a
// newline, so the user's command runs unchanged after it.
func applyWinPSUTF8(exe, command string) string {
	if !isWindowsPowerShell(exe) {
		return command
	}
	return winPSUTF8Prelude + "\n" + command
}
