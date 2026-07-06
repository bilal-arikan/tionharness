package api

import (
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/claudeauth"
)

// pendingLogins holds in-flight OAuth attempts between /start and /complete,
// keyed by an opaque flow id. Process-wide + short-lived (pruned after a TTL): the
// PKCE verifier must survive the round-trip to the browser but never reach the
// client. Guarded by its own mutex — independent of any workspace.
var pendingLogins = struct {
	mu sync.Mutex
	m  map[string]claudeauth.PendingLogin
}{m: map[string]claudeauth.PendingLogin{}}

const oauthLoginTTL = 10 * time.Minute

// prunePendingLogins drops attempts older than the TTL (called under the lock).
func prunePendingLogins(now time.Time) {
	for id, p := range pendingLogins.m {
		if now.Sub(p.CreatedAt) > oauthLoginTTL {
			delete(pendingLogins.m, id)
		}
	}
}

type oauthStartResp struct {
	FlowID  string `json:"flowId"`
	AuthURL string `json:"authUrl"`
}

// handleClaudeOAuthStart begins a Claude subscription (Max/Pro) login: it builds a
// fresh PKCE authorization URL, stashes the verifier server-side under a flow id and
// returns the URL for the popup to open. No secrets reach the client.
func (s *Server) handleClaudeOAuthStart(w http.ResponseWriter, r *http.Request) {
	authURL, pending, err := claudeauth.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "oauth begin failed: "+err.Error())
		return
	}
	flowID := uuid.NewString()
	pendingLogins.mu.Lock()
	prunePendingLogins(time.Now())
	pendingLogins.m[flowID] = pending
	pendingLogins.mu.Unlock()
	writeJSON(w, http.StatusOK, oauthStartResp{FlowID: flowID, AuthURL: authURL})
}

type oauthCompleteReq struct {
	FlowID string `json:"flowId"`
	Code   string `json:"code"` // the "<code>#<state>" the user pasted from the browser
}

type oauthCompleteResp struct {
	OK            bool   `json:"ok"`
	ClaudeHomeDir string `json:"claudeHomeDir"`
	ExpiresAt     int64  `json:"expiresAt"` // unix ms
	Subscription  string `json:"subscription,omitempty"`
}

// handleClaudeOAuthComplete finishes the login: it exchanges the pasted code (with
// the stashed PKCE verifier) for a credential and writes it into THIS workspace's
// claude-home, so the claude-cli provider authenticates on its next turn. The
// credential carries a refresh token, so the CLI keeps it fresh thereafter.
func (s *Server) handleClaudeOAuthComplete(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[oauthCompleteReq](w, r)
	if !ok {
		return
	}
	if req.FlowID == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "flowId and code are required")
		return
	}
	pendingLogins.mu.Lock()
	prunePendingLogins(time.Now())
	pending, found := pendingLogins.m[req.FlowID]
	if found {
		delete(pendingLogins.m, req.FlowID) // single-use
	}
	pendingLogins.mu.Unlock()
	if !found {
		writeError(w, http.StatusBadRequest, "login attempt expired or unknown — start over")
		return
	}

	cred, err := claudeauth.Exchange(nil, pending, req.Code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "token exchange failed: "+err.Error())
		return
	}

	home := filepath.Join(ws(r).DataDir, "claude-home")
	if err := claudeauth.WriteCredentials(home, cred); err != nil {
		writeError(w, http.StatusInternalServerError, "write credentials failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, oauthCompleteResp{
		OK:            true,
		ClaudeHomeDir: home,
		ExpiresAt:     cred.ExpiresAt,
		Subscription:  cred.SubscriptionType,
	})
}
