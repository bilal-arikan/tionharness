package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/market"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
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
		command = strings.ReplaceAll(command, "${CLAUDE_PLUGIN_ROOT}", dir)
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
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
