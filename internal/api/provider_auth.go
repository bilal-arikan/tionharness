package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type providerAuthTarget struct {
	kind    string
	homeDir string
	binPath string
}

type providerAuthDTO struct {
	Kind      string `json:"kind"`
	LoggedIn  bool   `json:"loggedIn"`
	Installed bool   `json:"installed"`
	HomeDir   string `json:"homeDir"`
	Tier      string `json:"tier,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func (s *Server) resolveAppCLIHome(w http.ResponseWriter, kindID string) (string, bool) {
	home, err := agent.ResolveCLIHomeDir(s.dataDir, kindID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return home, true
}

func (s *Server) resolveProviderAuthTarget(w http.ResponseWriter, r *http.Request, wantKind string) (providerAuthTarget, bool) {
	inst, ok := s.providerStore.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "provider instance not found")
		return providerAuthTarget{}, false
	}
	if inst.KindID != "claude-cli" && inst.KindID != "codex-cli" {
		writeError(w, http.StatusBadRequest, "provider instance is not a CLI kind")
		return providerAuthTarget{}, false
	}
	if wantKind != "" && inst.KindID != wantKind {
		writeError(w, http.StatusBadRequest, "provider auth route does not match instance kind")
		return providerAuthTarget{}, false
	}
	home, err := agent.ResolveCLIHomeDir(s.dataDir, inst.KindID, inst.Config[providers.FieldKeyConfigDir])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return providerAuthTarget{}, false
	}
	binPath := strings.TrimSpace(inst.Config[providers.FieldKeyCLIPath])
	if binPath == "" {
		if inst.KindID == "claude-cli" {
			binPath = s.providers.ClaudeCLIPath()
		} else {
			binPath = s.providers.CodexCLIPath()
		}
	}
	return providerAuthTarget{kind: inst.KindID, homeDir: home, binPath: binPath}, true
}

func (s *Server) handleProviderAuth(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "")
	if !ok {
		return
	}
	if target.kind == "claude-cli" {
		status := s.claudeAuthStatus(r, target.homeDir, target.binPath)
		writeJSON(w, http.StatusOK, providerAuthDTO{Kind: target.kind, LoggedIn: status.LoggedIn, Installed: status.Installed, HomeDir: target.homeDir, Detail: status.Detail})
		return
	}
	status := s.codexAuthStatus(r, target.homeDir, target.binPath)
	writeJSON(w, http.StatusOK, providerAuthDTO{Kind: target.kind, LoggedIn: status.LoggedIn, Installed: status.Installed, HomeDir: target.homeDir, Tier: codexSubscriptionTier(target.homeDir), Detail: status.Detail})
}

func (s *Server) handleProviderClaudeOAuthStart(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "claude-cli")
	if ok {
		s.handleClaudeOAuthStartFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderClaudeOAuthComplete(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "claude-cli")
	if ok {
		s.handleClaudeOAuthCompleteFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderClaudeOAuthLoopbackStart(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "claude-cli")
	if ok {
		s.handleClaudeOAuthLoopbackStartFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderClaudeOAuthLoopbackStatus(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "claude-cli")
	if ok {
		s.handleClaudeOAuthLoopbackStatusFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderCodexDeviceStart(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "codex-cli")
	if ok {
		s.handleCodexDeviceStartFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderCodexDeviceStatus(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "codex-cli")
	if ok {
		s.handleCodexDeviceStatusFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderCodexDeviceCancel(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "codex-cli")
	if ok {
		s.handleCodexDeviceCancelFor(w, r, target.homeDir, target.binPath)
	}
}

func (s *Server) handleProviderCodexAPIKeyLogin(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveProviderAuthTarget(w, r, "codex-cli")
	if ok {
		s.handleCodexAPIKeyLoginFor(w, r, target.homeDir, target.binPath)
	}
}
