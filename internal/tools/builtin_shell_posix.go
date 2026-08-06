package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// POSIX shell flavours the Bash tool may be backed by. The flavour decides how
// Windows paths must be spelled inside a command, which is NOT derivable from
// the OS alone — the same Windows 11 host reaches C:\ as /c/... under git-bash
// but /mnt/c/... under WSL. Callers use it to tell the agent which one it got.
const (
	POSIXShellNone    = ""        // no POSIX shell on this host (Bash tool not offered)
	POSIXShellUnix    = "unix"    // native /bin/sh
	POSIXShellGitBash = "gitbash" // git-bash on Windows: drives at /c/, /d/
	POSIXShellWSL     = "wsl"     // wsl.exe -e bash: drives at /mnt/c/, /mnt/d/
)

// POSIXShellFlavor reports which shell backs the Bash tool on this host. Returns
// POSIXShellNone when none is available.
func POSIXShellFlavor() string {
	exe, preArgs, ok := resolvePOSIXShell()
	if !ok {
		return POSIXShellNone
	}
	if runtime.GOOS != "windows" {
		return POSIXShellUnix
	}
	// The WSL route is the only one that goes through wsl.exe with a "-e bash"
	// prefix (see resolvePOSIXShell); everything else on Windows is a real bash.
	if len(preArgs) > 0 && strings.Contains(strings.ToLower(filepath.Base(exe)), "wsl") {
		return POSIXShellWSL
	}
	return POSIXShellGitBash
}

// resolvePOSIXShell finds the POSIX shell backing the Bash tool and the args that
// must precede "-c <command>". On Unix it is plain /bin/sh. On Windows it prefers
// a real git-bash and deliberately avoids C:\Windows\System32\bash.exe — see
// isWSLBashLauncher for why. Returns ok=false on Windows when no usable shell is
// found, so the Bash tool is simply not offered there (PowerShell is).
func resolvePOSIXShell() (exe string, preArgs []string, ok bool) {
	if runtime.GOOS != "windows" {
		return "/bin/sh", nil, true
	}
	// A bash on PATH is fine as long as it is not the WSL launcher.
	if p, found := lookInterpreter("bash"); found && !isWSLBashLauncher(p) {
		return p, nil, true
	}
	// Git ships bash.exe under bin/ and usr/bin/, but only Git\cmd (git.exe) lands
	// on PATH by default, so LookPath alone misses it on a stock install.
	if p, found := lookGitBash(); found {
		return p, nil, true
	}
	// WSL is all we have. Route through wsl.exe, which passes argv through
	// untouched, instead of the launcher that mangles it.
	if p, found := lookInterpreter("wsl"); found {
		return p, []string{"-e", "bash"}, true
	}
	return "", nil, false
}

// isWSLBashLauncher reports whether p is the Windows WSL launcher
// (C:\Windows\System32\bash.exe) rather than a real bash.
//
// The launcher is unusable as `bash.exe -c <script>`: it runs the payload through
// an OUTER shell before the real bash sees it, so the script is expanded twice.
// Measured on Windows 11 / WSL2 — sending `echo ver=$BASH_VERSION` makes bash
// report a syntax error quoting its own input as `echo ver=5.2.21(1)-release`,
// i.e. the substitution already happened upstream. Consequences: `$VAR` resolves
// against the outer shell (empty for anything the script itself assigned) and
// `$(...)` runs one step too early, so `T=$(cat tok.txt); curl -H "Bearer $T"`
// silently sends an empty credential. `wsl.exe -e bash -c <script>` is unaffected.
func isWSLBashLauncher(p string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	lower := strings.ToLower(filepath.ToSlash(p))
	for _, dir := range []string{"/system32/", "/syswow64/", "/sysnative/"} {
		if strings.HasSuffix(lower, dir+"bash.exe") {
			return true
		}
	}
	return false
}

// lookGitBash locates git-bash without relying on PATH: first by deriving it from
// a git.exe on PATH (<git>\cmd\git.exe -> <git>\bin\bash.exe), then from the
// standard install locations. Git-bash is the preferred Windows shell because it
// shares the Windows filesystem and network stack — unlike WSL, whose separate
// namespace makes a Windows path or a 127.0.0.1 service unreachable from inside.
var lookGitBash = func() (string, bool) {
	var roots []string
	if git, err := exec.LookPath("git"); err == nil {
		// <root>\cmd\git.exe or <root>\bin\git.exe
		roots = append(roots, filepath.Dir(filepath.Dir(git)))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		roots = append(roots, filepath.Join(pf, "Git"))
	}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		roots = append(roots, filepath.Join(pf, "Git"))
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		roots = append(roots, filepath.Join(la, "Programs", "Git"))
	}
	for _, root := range roots {
		for _, rel := range []string{`bin\bash.exe`, `usr\bin\bash.exe`} {
			p := filepath.Join(root, rel)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
		}
	}
	return "", false
}
