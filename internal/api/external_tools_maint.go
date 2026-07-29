package api

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Maintenance actions for the optional token-optimizer CLIs (rtk / sqz).
//
// These are ACTIONS, not settings. The tools' own configuration files are
// MACHINE-GLOBAL while TionSwarm settings are per-workspace, so mirroring
// config.toml keys into the settings screen would quietly promise a scope the
// setting cannot honour — changing it in one workspace would change every other.
// Instead the panel exposes: the report both tools already produce, the one reset
// sqz itself recommends, and a way to open rtk's config file where it actually
// lives.
//
// Security: every command below is a FIXED argv. No request field reaches a shell
// or an argument — the endpoints take no parameters at all — so there is nothing
// to inject through.

// toolCmdTimeout bounds a maintenance subprocess. These are local, near-instant
// commands; the timeout only stops a wedged binary from holding a request open.
const toolCmdTimeout = 15 * time.Second

// ansiEscapeRe matches SGR/CSI escape sequences. Both tools colourise their
// reports even when stdout is a pipe (`sqz gain` emits bar charts wrapped in
// colour codes), and those bytes render as literal "[1m[36m" noise inside the
// panel's <pre>. Stripping here keeps the transport clean for every consumer
// rather than making each one re-derive the same regex.
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// runToolCmd executes a fixed argv and returns its combined output. A missing
// binary is reported as absent rather than as an error: the panel renders "not
// installed" for it, which is a normal state, not a failure.
func runToolCmd(ctx context.Context, name string, args ...string) (out string, found bool) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	runCtx, cancel := context.WithTimeout(ctx, toolCmdTimeout)
	defer cancel()
	c := exec.CommandContext(runCtx, bin, args...)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	// Exit status is ignored on purpose: `rtk gain` and friends report useful text
	// on stdout even when they exit non-zero (rtk exits 3 on some success paths —
	// see internal/agent/rtk_optimizer.go). The OUTPUT is the product here.
	_ = c.Run()
	return strings.TrimRight(ansiEscapeRe.ReplaceAllString(buf.String(), ""), "\r\n"), true
}

// tokenToolReport is the "token savings" panel payload: each tool's own report,
// verbatim, plus rtk's config location so the UI can offer to open it.
type tokenToolReport struct {
	RtkFound      bool   `json:"rtkFound"`
	RtkGain       string `json:"rtkGain,omitempty"`
	SqzFound      bool   `json:"sqzFound"`
	SqzGain       string `json:"sqzGain,omitempty"`
	RtkConfigPath string `json:"rtkConfigPath,omitempty"`
	// RtkConfigExists is false until the user runs `rtk config --create`; rtk works
	// off built-in defaults until then, so "missing" is normal, not broken.
	RtkConfigExists bool `json:"rtkConfigExists"`
}

// rtkConfigPath is where rtk keeps config.toml: %APPDATA%\rtk\config.toml on
// Windows, and the XDG config dir elsewhere. Matches what `rtk config` prints.
func rtkConfigPath() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "rtk", "config.toml")
		}
		return ""
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "rtk", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "rtk", "config.toml")
}

// handleTokenToolReport returns each installed optimizer's own savings report.
// TionSwarm does not compute these numbers — it shows what the tools report, so
// the figures cannot drift from the tools' own accounting.
func (s *Server) handleTokenToolReport(w http.ResponseWriter, r *http.Request) {
	rep := tokenToolReport{RtkConfigPath: rtkConfigPath()}
	rep.RtkGain, rep.RtkFound = runToolCmd(r.Context(), "rtk", "gain")
	rep.SqzGain, rep.SqzFound = runToolCmd(r.Context(), "sqz", "gain")
	if rep.RtkConfigPath != "" {
		if _, err := os.Stat(rep.RtkConfigPath); err == nil {
			rep.RtkConfigExists = true
		}
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleSqzResetCache clears sqz's dedup cache. sqz's own help recommends exactly
// this "if stale `§ref:HASH§` tokens are confusing the agent" — a real failure
// mode, since a dedup hit replaces a whole command result with a bare pointer.
// Only the CACHE is cleared; accumulated stats and session history are kept.
func (s *Server) handleSqzResetCache(w http.ResponseWriter, r *http.Request) {
	out, found := runToolCmd(r.Context(), "sqz", "reset", "--cache-only", "-y")
	if !found {
		writeError(w, http.StatusNotFound, "sqz PATH'te bulunamadı")
		return
	}
	s.logger.Info("sqz dedup cache cleared from settings")
	writeJSON(w, http.StatusOK, map[string]string{"output": out})
}

// handleRevealRtkConfig opens rtk's config file in the OS file manager. It does
// NOT create the file: rtk runs on built-in defaults until `rtk config --create`
// is run, and silently materialising a config from here would change how rtk
// behaves for every tool on this machine, from a screen that is scoped to one
// workspace. When the file is absent the containing folder is opened instead.
func (s *Server) handleRevealRtkConfig(w http.ResponseWriter, r *http.Request) {
	path := rtkConfigPath()
	if path == "" {
		writeError(w, http.StatusNotFound, "rtk config yolu bu platformda çözülemedi")
		return
	}
	target := "/select," + path
	if _, err := os.Stat(path); err != nil {
		dir := filepath.Dir(path)
		if _, derr := os.Stat(dir); derr != nil {
			writeError(w, http.StatusNotFound, "rtk config klasörü yok — önce `rtk config --create` çalıştırın")
			return
		}
		target = dir
	}
	// Detached from r.Context(): a fire-and-forget launch must not be killed when
	// the handler returns. explorer.exe returns non-zero even on success.
	if err := exec.Command("explorer.exe", target).Start(); err != nil {
		s.logger.Warn("reveal rtk config failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}
