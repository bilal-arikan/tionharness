package proc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// pathMarker brackets $PATH in the login shell's output so banner text printed
// by rc files (motd, fortune, plugin managers) cannot be mistaken for it.
const pathMarker = "__TIONHARNESS_PATH__"

// AugmentPATH widens this process's PATH on macOS/Linux so child processes —
// the claude/codex CLIs, node/npx MCP servers, rg, git, ffmpeg — are found the
// same way a terminal finds them. An app started from Finder, the Dock, launchd or
// a desktop launcher inherits a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin) that
// hides Homebrew, ~/.local/bin, nvm and npm's global prefix.
//
// It merges, in order: the user's login+interactive shell PATH (skipped when this
// process already runs attached to a terminal, whose PATH is the real one), the
// current PATH, then well-known install dirs that exist. Existing entries keep
// their position; only missing dirs are added. TIONHARNESS_NO_PATH_AUGMENT=1
// turns it off. No-op on Windows. Returns the dirs that were added.
func AugmentPATH() []string {
	if runtime.GOOS == "windows" || os.Getenv("TIONHARNESS_NO_PATH_AUGMENT") == "1" {
		return nil
	}
	current := os.Getenv("PATH")
	var shellPath string
	if !stdinIsTerminal() {
		shellPath = loginShellPATH(3 * time.Second)
	}
	home, _ := os.UserHomeDir()
	merged, added := mergePATH(shellPath, current, existingDirs(wellKnownBinDirs(runtime.GOOS, home)))
	if len(added) > 0 {
		_ = os.Setenv("PATH", merged)
	}
	return added
}

// mergePATH joins the shell PATH, the current PATH and extra dirs, dropping
// duplicates and empty entries. The current PATH's entries all survive; added
// lists what was not already in current.
func mergePATH(shellPath, current string, extra []string) (merged string, added []string) {
	have := map[string]bool{}
	for _, d := range filepath.SplitList(current) {
		have[filepath.Clean(d)] = true
	}
	seen := map[string]bool{}
	var out []string
	push := func(d string) {
		if d == "" {
			return
		}
		c := filepath.Clean(d)
		if seen[c] {
			return
		}
		seen[c] = true
		out = append(out, d)
		if !have[c] {
			added = append(added, d)
		}
	}
	for _, d := range filepath.SplitList(shellPath) {
		push(d)
	}
	for _, d := range filepath.SplitList(current) {
		push(d)
	}
	for _, d := range extra {
		push(d)
	}
	return strings.Join(out, string(os.PathListSeparator)), added
}

// wellKnownBinDirs are the usual per-user and package-manager bin dirs on goos.
func wellKnownBinDirs(goos, home string) []string {
	var dirs []string
	switch goos {
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin", "/snap/bin")
	}
	if home != "" {
		for _, rel := range []string{".local/bin", ".npm-global/bin", ".bun/bin", ".volta/bin", ".cargo/bin", "go/bin"} {
			dirs = append(dirs, filepath.Join(home, filepath.FromSlash(rel)))
		}
	}
	return dirs
}

func existingDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// loginShellPATH asks the user's shell for the PATH a new terminal would get
// (-i -l -c reads both login and interactive rc files, where Homebrew and nvm
// put their PATH lines). Any failure or timeout yields "".
func loginShellPATH(timeout time.Duration) string {
	shell := os.Getenv("SHELL")
	if shell == "" || !filepath.IsAbs(shell) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := CommandContext(ctx, shell, "-i", "-l", "-c", `printf '\n`+pathMarker+`%s`+pathMarker+`\n' "$PATH"`)
	TreeKill(cmd)
	// An rc file may start a daemon that inherits stdout; don't wait on it.
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Stdin = nil
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return ""
	}
	out := stdout.String()
	start := strings.Index(out, pathMarker)
	if start < 0 {
		return ""
	}
	rest := out[start+len(pathMarker):]
	end := strings.Index(rest, pathMarker)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
