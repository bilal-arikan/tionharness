package tools

import (
	"os"
	"runtime"
)

// scriptEnvAllowlist returns the names of the ONLY environment variables passed
// through to a transform_data subprocess. It is an ALLOWLIST (not a denylist) so
// secrets that happen to live in the parent process env — ANTHROPIC_API_KEY and
// any other credential — never reach the script. Only what an interpreter needs
// to start and find its standard library is included.
func scriptEnvAllowlist() []string {
	if runtime.GOOS == "windows" {
		return []string{
			"PATH", "Path", "PATHEXT",
			"SystemRoot", "SystemDrive", "WINDIR",
			"TEMP", "TMP",
			"APPDATA", "LOCALAPPDATA", "PROGRAMDATA",
			"PROGRAMFILES", "ProgramFiles(x86)", "PROGRAMW6432",
			"USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		}
	}
	return []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "USER", "SHELL"}
}

// minimalScriptEnv builds the environment for a transform_data subprocess: the
// allowlisted host vars that are actually set, plus a couple of interpreter
// hygiene settings. Everything else (including every secret) is dropped.
func minimalScriptEnv() []string {
	env := make([]string, 0, 16)
	for _, k := range scriptEnvAllowlist() {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	// Keep Python from writing .pyc files into the working tree and force
	// unbuffered stdio so captured output is complete and ordered.
	env = append(env, "PYTHONDONTWRITEBYTECODE=1", "PYTHONUNBUFFERED=1")
	return env
}
