package agent

import (
	"runtime"
	"strings"
)

// cliHookCommand adapts a TionSwarm hook command to the interpreter Claude Code
// will actually spawn it with.
//
// The two engines disagree about the shell, exactly the way cliMatcherRegex's two
// engines disagree about the matcher dialect:
//
//   - TionSwarm's own execHook runs a hook through powershell.exe on Windows and
//     /bin/sh elsewhere, so a Windows workspace's hooks are authored in PowerShell.
//   - Claude Code runs every hook command through a POSIX shell (/usr/bin/bash),
//     on Windows too.
//
// Passing a PowerShell one-liner verbatim therefore hands bash a script it cannot
// parse. Observed live: the rtk optimizer hook
// (`$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; …`) died with
// `syntax error near unexpected token '|'` on EVERY Bash call of a claude-cli
// turn — and because a failing PreToolUse hook was treated as a refusal by the
// CLI, the Bash tool stayed unusable for the rest of the session.
//
// Fix: on Windows, wrap the command so the interpreter matches the dialect it was
// written in — `powershell.exe -NoProfile -NonInteractive -Command "<cmd>"`, which
// is valid bash AND re-enters PowerShell for the body. Commands that already
// invoke an interpreter explicitly (the `powershell -File …` / `sqz hook claude`
// forms) are left alone: they are plain executable invocations that bash parses
// fine, and double-wrapping would break their own quoting.
//
// Stdin is inherited by the wrapped process, so the hook payload still reaches the
// script — the contract execHook and the CLI share.
func cliHookCommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" || runtime.GOOS != "windows" {
		return command
	}
	if !needsPowerShellWrap(cmd) {
		return command
	}
	return "powershell.exe -NoProfile -NonInteractive -Command " + posixSingleQuote(cmd)
}

// needsPowerShellWrap reports whether a command is PowerShell source that bash
// would choke on, rather than a plain executable invocation bash can run as-is.
//
// The discriminator is the first token: a command that starts by invoking a
// program (`sqz hook claude`, `powershell -File x.ps1`, `node hook.js`) is
// interpreter-agnostic and must pass through untouched. A command that starts
// with PowerShell syntax — a `$variable` assignment, a `[Type]::Member` call, an
// `&`/`.` call operator, or a cmdlet-style `Verb-Noun` — is PowerShell source.
func needsPowerShellWrap(cmd string) bool {
	first := cmd
	if i := strings.IndexAny(first, " \t"); i >= 0 {
		first = first[:i]
	}
	switch {
	case strings.HasPrefix(first, "$"), // $j = ...
		strings.HasPrefix(first, "["), // [Console]::In...
		strings.HasPrefix(first, "&"), // & 'C:\path with space\x.ps1'
		strings.HasPrefix(first, "."): // . .\profile.ps1 (dot-source)
		return true
	}
	// Verb-Noun cmdlet (Get-Content, ConvertFrom-Json, …): a hyphenated single
	// token with no path separator and no extension. `rtk-wrapper.sh` has an
	// extension; `some/dir-name` has a separator; both stay unwrapped.
	if strings.Contains(first, "-") &&
		!strings.ContainsAny(first, `/\.`) &&
		isVerbNoun(first) {
		return true
	}
	return false
}

// isVerbNoun reports whether a token looks like a PowerShell cmdlet name: exactly
// two segments around a single hyphen, each starting with an upper-case letter.
func isVerbNoun(tok string) bool {
	parts := strings.Split(tok, "-")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		c := p[0]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// posixSingleQuote wraps s in single quotes for a POSIX shell, escaping embedded
// single quotes the only way sh allows ('\” — close, escaped quote, reopen). The
// result is a single bash word, so the PowerShell body inside is passed to
// powershell.exe untouched no matter what metacharacters ($, |, ;) it contains.
func posixSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
