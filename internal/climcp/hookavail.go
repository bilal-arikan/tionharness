package climcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// HookExecutableMissing reports the program a command hook would start when
// that program cannot be found on PATH, so the hook can be left out of the CLI
// settings instead of failing on every matching tool call. Measured 2026-10-23
// (WS3 SES745): a PreToolUse `sqz hook claude` hook with sqz not installed failed
// with exit 127 before every Bash call and put an error line into the
// transcript each time (~70 in one turn).
//
// It only answers "missing" when it is sure: shell syntax, builtins, relative
// paths and PowerShell-authored commands all count as available.
func HookExecutableMissing(command string) (string, bool) {
	cmd := strings.TrimSpace(command)
	if cmd == "" || needsPowerShellWrap(cmd) {
		return "", false
	}
	var prog string
	for _, tok := range strings.Fields(cmd) {
		if isEnvAssignment(tok) {
			continue
		}
		prog = strings.Trim(tok, `"'`)
		break
	}
	if prog == "" || strings.ContainsAny(prog, "()${}`;|&<>*?[]=") || shellBuiltins[prog] {
		return "", false
	}
	if strings.Contains(prog, "/") || strings.Contains(prog, `\`) {
		if strings.HasPrefix(prog, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				prog = filepath.Join(home, prog[2:])
			}
		}
		if !filepath.IsAbs(prog) {
			return "", false // relative to the CLI's cwd; cannot judge here
		}
		if _, err := os.Stat(prog); err != nil {
			return prog, true
		}
		return "", false
	}
	if _, err := exec.LookPath(prog); err != nil {
		return prog, true
	}
	return "", false
}

func isEnvAssignment(tok string) bool {
	i := strings.IndexByte(tok, '=')
	if i <= 0 {
		return false
	}
	for _, r := range tok[:i] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

var shellBuiltins = map[string]bool{
	"if": true, "then": true, "for": true, "while": true, "case": true, "test": true,
	"echo": true, "printf": true, "true": true, "false": true, "exit": true, "cd": true,
	"export": true, "exec": true, "eval": true, "source": true, ".": true, ":": true,
	"command": true, "set": true, "read": true, "!": true,
}

// missingHookWarned dedupes the skip warning per command: settings are written
// every turn, so one missing tool must not flood the log.
var missingHookWarned sync.Map
