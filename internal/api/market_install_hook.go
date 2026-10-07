package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/fspath"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// installHookPack registers a lifecycle/tool hook imported from a foreign plugin
// (KindHook). Any bundled scripts (Pack.Files) are materialised under the
// workspace's hook-scripts dir and the ${CLAUDE_PLUGIN_ROOT} placeholder in the
// command is rewritten to that directory, so an imported caveman-style hook runs
// against local copies of its scripts. The hook is created enabled and
// user-owned (CreatedBy "") — the native tool loop / turn orchestrator picks it
// up on the next turn.
func (s *Server) installHookPack(r *http.Request, wsp *workspace.Workspace, pack market.Pack) (market.InstallResult, error) {
	hp := pack.Payload.Hook
	if hp == nil || strings.TrimSpace(hp.Command) == "" {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "hook pack is missing its payload"}
	}
	if !db.ValidHookEvent(hp.Event) {
		return market.InstallResult{}, httpErr{http.StatusBadRequest, "unsupported hook event: " + hp.Event}
	}

	command := hp.Command
	if len(pack.Files) > 0 {
		base := wsp.Runtime.WorkspaceHookScriptsDir()
		if base == "" {
			return market.InstallResult{}, httpErr{http.StatusInternalServerError, "workspace has no hook-scripts directory"}
		}
		slug := strings.TrimPrefix(pack.ID, market.KindHook+".")
		if slug == "" {
			slug = "hook"
		}
		dir := filepath.Join(base, slug)
		if err := materialiseHookScripts(dir, pack.Files); err != nil {
			return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
		}
		command = substitutePluginRoot(command, dir, runtime.GOOS)
	}

	hook, err := wsp.DB.CreateHook(r.Context(), db.Hook{
		Event:      hp.Event,
		Matcher:    hp.Matcher,
		Type:       "command",
		Command:    command,
		TimeoutSec: hp.TimeoutSec,
		Enabled:    true,
	})
	if err != nil {
		return market.InstallResult{}, httpErr{http.StatusInternalServerError, err.Error()}
	}
	s.logger.Info("hook pack installed", "id", hook.ID, "event", hook.Event)
	return market.InstallResult{
		Kind: market.KindHook, Ref: hook.ID,
		Message: "Hook (" + hp.Event + ") installed",
	}, nil
}

// materialiseHookScripts writes a pack's bundled script files under dir, keyed by
// their relative paths. Each path is validated (no absolute, no "..") so a
// malicious pack cannot escape the target directory.
func materialiseHookScripts(dir string, files map[string][]byte) error {
	for rel, data := range files {
		clean := filepath.ToSlash(strings.TrimSpace(rel))
		if clean == "" || strings.HasPrefix(clean, "/") || filepath.IsAbs(clean) || strings.Contains(clean, "..") {
			continue // reject unsafe paths rather than escape the sandbox
		}
		dst := filepath.Join(dir, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, fspath.FileModeFor(data)); err != nil {
			return err
		}
	}
	return nil
}

const pluginRootVar = "${CLAUDE_PLUGIN_ROOT}"

// substitutePluginRoot replaces ${CLAUDE_PLUGIN_ROOT} in a hook-pack command with
// the directory the pack's scripts were materialised in. A plain textual replace
// breaks as soon as that directory needs quoting — a data dir under
// "~/Library/Application Support" or C:\Users\Ad Soyad — because the shell then
// splits `/x y/hooks/run.sh` into two words. So when dir is not shell-safe, each
// occurrence is quoted for the shell the hook runs in (bash off Windows,
// PowerShell on Windows), according to its position:
//
//   - unquoted: the whole word (placeholder + the path suffix glued to it) is
//     single-quoted; on Windows a quoted word that STARTS the command is prefixed
//     with the call operator (& '...') or PowerShell would just echo the string;
//   - inside "...": dir is inserted with the characters special there escaped;
//   - inside '...': dir is inserted with embedded single quotes escaped.
//
// A shell-safe dir (the common case) is inserted verbatim, exactly as before.
func substitutePluginRoot(command, dir, goos string) string {
	if !strings.Contains(command, pluginRootVar) {
		return command
	}
	if !needsShellQuoting(dir, goos) {
		return strings.ReplaceAll(command, pluginRootVar, dir)
	}
	windows := goos == "windows"
	var b strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(command); {
		if strings.HasPrefix(command[i:], pluginRootVar) {
			switch {
			case inSingle:
				if windows {
					b.WriteString(strings.ReplaceAll(dir, "'", "''"))
				} else {
					b.WriteString(strings.ReplaceAll(dir, "'", `'\''`))
				}
				i += len(pluginRootVar)
			case inDouble:
				b.WriteString(escapeInDoubleQuotes(dir, windows))
				i += len(pluginRootVar)
			default:
				end := i + len(pluginRootVar)
				for end < len(command) && !strings.ContainsRune(" \t\r\n;|&<>()'\"", rune(command[end])) {
					end++
				}
				word := dir + command[i+len(pluginRootVar):end]
				if windows {
					if strings.TrimSpace(command[:i]) == "" {
						b.WriteString("& ")
					}
					b.WriteString("'" + strings.ReplaceAll(word, "'", "''") + "'")
				} else {
					b.WriteString("'" + strings.ReplaceAll(word, "'", `'\''`) + "'")
				}
				i = end
			}
			continue
		}
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// needsShellQuoting reports whether dir contains a character its shell would
// split or interpret when left unquoted. Backslash is a plain path character on
// Windows only.
func needsShellQuoting(dir, goos string) bool {
	special := " \t'\"$`&;|<>()*?[]{}#~!"
	if goos != "windows" {
		special += `\`
	}
	return strings.ContainsAny(dir, special)
}

// escapeInDoubleQuotes escapes the characters that stay special inside "..." —
// $ ` " \ for bash, $ ` " for PowerShell (whose escape character is the backtick).
func escapeInDoubleQuotes(s string, windows bool) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case windows && (r == '$' || r == '`' || r == '"'):
			b.WriteRune('`')
		case !windows && (r == '$' || r == '`' || r == '"' || r == '\\'):
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
