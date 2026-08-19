package api

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/codexauth"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// codexAuthManager tracks in-flight device-auth flows across all workspaces,
// keyed internally by CODEX_HOME (see codexauth.Manager) so at most one flow
// runs per workspace's codex-home at a time. Process-wide, independent of any
// single request — a flow started by one request is polled/cancelled by
// later ones.
var codexAuthManager = codexauth.NewManager()

// codexHomeDirFor mirrors agent.workspaceCodexHomeDir: <workspace>/codex-home,
// a sibling of <workspace>/workspace. Reusing ws(r).DataDir (the same field
// handleWorkspaceClaudeAuth uses for claude-home) keeps this the single
// resolution rule instead of a second, possibly-drifting one.
func codexHomeDirFor(r *http.Request) string {
	return filepath.Join(ws(r).DataDir, "codex-home")
}

type codexAuthDTO struct {
	LoggedIn bool `json:"loggedIn"`
	// Installed reports whether the codex CLI binary is resolvable at all —
	// same distinction as claudeAuthDTO.Installed.
	Installed    bool   `json:"installed"`
	CodexHomeDir string `json:"codexHomeDir"`
	Detail       string `json:"detail,omitempty"`
}

// handleWorkspaceCodexAuth reports whether this workspace's codex-home is
// logged in. Two stages, cheapest first: the on-disk credential check rejects
// an empty/absent auth.json without spawning anything, then a live probe turn
// proves the credential actually works. The disk check alone reported "logged
// in" for a codex-home whose token was expired or revoked — every real turn
// then failed with 401, so presence is treated as necessary, not sufficient.
func (s *Server) handleWorkspaceCodexAuth(w http.ResponseWriter, r *http.Request) {
	home := codexHomeDirFor(r)
	binPath := s.providers.CodexCLIPath()
	installed := binPath != ""
	if !installed {
		writeJSON(w, http.StatusOK, codexAuthDTO{CodexHomeDir: home,
			Detail: "codex CLI not found (set its path in Providers, or use an API-key provider)"})
		return
	}
	if codexSubscriptionTier(home) == "" {
		writeJSON(w, http.StatusOK, codexAuthDTO{Installed: true, CodexHomeDir: home,
			Detail: "not logged in"})
		return
	}
	p, err := s.providers.Get("codex-cli")
	if err != nil {
		writeJSON(w, http.StatusOK, codexAuthDTO{Installed: true, CodexHomeDir: home, Detail: err.Error()})
		return
	}
	cli, ok := providers.AsCLI(p)
	if !ok {
		writeError(w, http.StatusInternalServerError, "codex-cli provider unavailable")
		return
	}
	prober, ok := cli.(providers.AuthProber)
	if !ok {
		writeError(w, http.StatusInternalServerError, "codex-cli provider cannot probe authentication")
		return
	}
	cli.SetConfigDir(home)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if perr := prober.ProbeAuth(ctx); perr != nil {
		writeJSON(w, http.StatusOK, codexAuthDTO{Installed: true, CodexHomeDir: home, Detail: perr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, codexAuthDTO{LoggedIn: true, Installed: true, CodexHomeDir: home})
}

type codexDeviceStartResp struct {
	VerifyURL    string `json:"verifyUrl"`
	Code         string `json:"code"`
	ExpiresInSec int    `json:"expiresInSec"`
}

// codexDeviceExpirySec matches the CLI's own stated code lifetime (see the
// device prompt: "expires in 15 minutes"), surfaced to the client so it can
// show a countdown instead of guessing one.
const codexDeviceExpirySec = 15 * 60

// handleCodexDeviceStart begins a `codex login --device-auth` flow against
// this workspace's codex-home and returns the verification URL + one-time
// code for the UI to display. The subprocess keeps running in the background
// until the user approves in their browser, the code expires, or the flow is
// cancelled.
func (s *Server) handleCodexDeviceStart(w http.ResponseWriter, r *http.Request) {
	binPath := s.providers.CodexCLIPath()
	if binPath == "" {
		writeError(w, http.StatusBadRequest, "codex CLI not found (set its path in Providers)")
		return
	}
	home := codexHomeDirFor(r)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	flow, err := codexAuthManager.Start(ctx, binPath, home)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, codexDeviceStartResp{
		VerifyURL:    flow.VerifyURL(),
		Code:         flow.Code(),
		ExpiresInSec: codexDeviceExpirySec,
	})
}

type codexDeviceStatusResp struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// handleCodexDeviceStatus reports the current state of the workspace's
// in-flight (or just-finished) device-auth flow. 404 when no flow has been
// started for this codex-home since the process started.
func (s *Server) handleCodexDeviceStatus(w http.ResponseWriter, r *http.Request) {
	home := codexHomeDirFor(r)
	flow, ok := codexAuthManager.Flow(home)
	if !ok {
		writeError(w, http.StatusNotFound, "no device-auth flow found for this workspace")
		return
	}
	state, errMsg := flow.State()
	writeJSON(w, http.StatusOK, codexDeviceStatusResp{State: string(state), Error: errMsg})
}

// handleCodexDeviceCancel cancels the workspace's in-flight device-auth flow,
// if any. Idempotent: cancelling an already-terminal or nonexistent flow is
// not an error.
func (s *Server) handleCodexDeviceCancel(w http.ResponseWriter, r *http.Request) {
	home := codexHomeDirFor(r)
	if flow, ok := codexAuthManager.Flow(home); ok {
		flow.Cancel()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type codexAPIKeyReq struct {
	APIKey string `json:"apiKey"`
}

// handleCodexAPIKeyLogin runs `codex login --with-api-key` against this
// workspace's codex-home, piping the key over stdin (never argv) so it never
// appears in a process listing. The key is never echoed back in the response.
func (s *Server) handleCodexAPIKeyLogin(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[codexAPIKeyReq](w, r)
	if !ok {
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "apiKey is required")
		return
	}
	binPath := s.providers.CodexCLIPath()
	if binPath == "" {
		writeError(w, http.StatusBadRequest, "codex CLI not found (set its path in Providers)")
		return
	}
	home := codexHomeDirFor(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := codexauth.LoginWithAPIKey(ctx, binPath, home, req.APIKey); err != nil {
		writeError(w, http.StatusBadGateway, "codex login failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
