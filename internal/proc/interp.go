package proc

import (
	"os/exec"
	"runtime"
	"strings"
)

// Interpreter resolution lives here — in the leaf package both the tool layer and
// the external-tools catalog already depend on — so the two can never disagree
// about WHICH python they mean. A panel that reports the version of a different
// binary than run_code executes is worse than no panel at all.

// PythonCandidates returns the interpreter names to try, most-likely first.
//
// On Windows real CPython installs as python.exe; python3.exe usually exists ONLY
// as a Microsoft Store "app execution alias" stub, so the order is flipped there.
func PythonCandidates() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

// LookInterpreter returns the first candidate found on PATH, skipping Windows
// "app execution alias" stubs under WindowsApps — those are 0-byte reparse points
// that, when run with a stripped env, print "Python was not found" and exit 9009
// instead of executing a real interpreter.
func LookInterpreter(candidates ...string) (string, bool) {
	for _, c := range candidates {
		p, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		if IsWindowsAppAlias(p) {
			continue
		}
		return p, true
	}
	return "", false
}

// IsWindowsAppAlias reports whether path is a Microsoft Store app-execution-alias
// stub rather than a real executable.
func IsWindowsAppAlias(path string) bool {
	return runtime.GOOS == "windows" && strings.Contains(strings.ToLower(path), `\windowsapps\`)
}
