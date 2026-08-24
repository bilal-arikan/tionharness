package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// --- shell-environment capability -------------------------------------------
//
// Which POSIX shell backs the Bash tool is NOT derivable from the OS: on Windows
// it may be git-bash (Windows drives at /c/) or WSL (/mnt/c/). An agent that
// guesses wrong burns a full round-trip per attempt on "No such file or
// directory" before it works the mount layout out from scratch — measured in
// production traces as the FIRST THREE shell calls of a Windows session, every
// session, because nothing in the prompt says which one it got.
//
// This capability states the resolved flavour once, in the cached static prefix.

var shellEnvironmentCapability = Capability{
	ID: "shell-environment",
	Detect: func(ctx context.Context, r *Runtime, _ db.Agent) bool {
		// Only worth a prompt block when the agent can actually run shell commands
		// AND the path spelling is ambiguous (Windows). A native Unix /bin/sh needs
		// no explanation.
		return r.tun.ShellEnabled() && shellEnvironmentGuidance(tools.POSIXShellFlavor()) != ""
	},
	Context: func(ctx context.Context, r *Runtime, cwd string) string {
		return shellEnvironmentGuidance(tools.POSIXShellFlavor())
	},
}

// shellEnvironmentGuidance renders the block for a resolved shell flavour.
// Returns "" when there is nothing worth saying (native Unix, or no POSIX shell
// at all — in which case the Bash tool is not offered either).
func shellEnvironmentGuidance(flavor string) string {
	switch flavor {
	case tools.POSIXShellGitBash:
		return "# Shell environment\n" +
			"The `Bash` tool is backed by **git-bash on Windows**, not WSL and not a Linux box:\n" +
			"- Windows drives are mounted at `/c/`, `/d/` — there is NO `/mnt/c`. Use `/c/Users/...`; " +
			"a `/mnt/c/...` path fails with \"No such file or directory\".\n" +
			"- Windows absolute paths (`C:\\Users\\...`) work in the `PowerShell` tool and in the file " +
			"tools, but NOT as a bare argument inside a git-bash command line.\n" +
			"- It runs Windows executables, so tools installed for Windows are called by their real " +
			"names (`cargo.exe`, `python.exe`); a bare name only resolves if it is on the Windows PATH.\n" +
			"- The filesystem and network stack are shared with Windows: a `127.0.0.1` service and any " +
			"Windows path are reachable directly."
	case tools.POSIXShellWSL:
		return "# Shell environment\n" +
			"The `Bash` tool is backed by **WSL** (`wsl.exe -e bash`), a SEPARATE Linux namespace:\n" +
			"- Windows drives are mounted at `/mnt/c/`, `/mnt/d/` — `/c/...` does not exist.\n" +
			"- The Linux filesystem is not the Windows one: software installed on Windows " +
			"(and its PATH) is not visible here; install/run Linux builds instead.\n" +
			"- A service listening on the Windows host's `127.0.0.1` is NOT reachable from inside " +
			"WSL under that address. Use the `PowerShell` tool for anything Windows-native."
	default:
		// Native Unix (or no POSIX shell): the layout is exactly what the model
		// already assumes, so an extra prompt block would be pure token cost.
		return ""
	}
}
