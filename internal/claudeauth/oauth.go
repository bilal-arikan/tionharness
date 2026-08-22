// Package claudeauth implements the Claude Code subscription (Max/Pro) OAuth
// authorization-code + PKCE flow directly, so TionSwarm can log a workspace's
// isolated claude-home in from an in-app popup — without shelling out to the
// interactive `claude setup-token` / `claude auth login` TUI.
//
// Flow (manual-code-paste variant, as the CLI's own subscription login uses):
//  1. Begin() builds the authorization URL (with a fresh PKCE verifier + state)
//     and returns it plus the pending state to stash server-side.
//  2. The user opens the URL, authorizes with their Max/Pro account, and copies
//     the "<code>#<state>" shown on the callback page.
//  3. Exchange() posts that code + the stashed verifier to the token endpoint and
//     returns the resulting credential (access + refresh token + expiry).
//  4. WriteCredentials() persists it as <home>/.credentials.json in the exact
//     shape Claude Code reads (claudeAiOauth{...}), so the CLI picks it up with no
//     interactive login — and refreshes it itself thereafter (refresh token).
//
// The OAuth constants below were extracted from the installed claude binary
// (v2.1.201, 2026-07): the public client_id, the claude.com/platform.claude.com
// endpoints and the credential file shape. They are version-sensitive — if a
// future CLI rotates them, update THIS block (and nothing else).
package claudeauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/bilal-arikan/tionswarm/internal/textutil"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OAuth client constants (Claude Code subscription login). Extracted from the
// installed CLI binary + the public client-metadata document. Single source of
// truth: change here if the CLI rotates them.
const (
	clientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	authorizeURL = "https://claude.com/cai/oauth/authorize"
	tokenURL     = "https://platform.claude.com/v1/oauth/token"
	redirectURI  = "https://platform.claude.com/oauth/code/callback"
	// scopes requested — matches what a working subscription credential stores
	// (space-separated on the wire). user:inference is the one that actually
	// authorises model calls; the rest mirror the CLI's own request.
	scopes = "user:inference user:profile user:sessions:claude_code user:mcp_servers user:file_upload"
)

// FlowConfig selects the OAuth client identity + redirect target for an attempt.
// Two presets: ManualConfig (paste "<code>#<state>") and LoopbackConfig (browser
// redirects to a local listener — no paste).
type FlowConfig struct {
	ClientID    string
	RedirectURI string
}

// ManualConfig is the paste-the-code flow: the UUID client + the platform callback
// page that displays "<code>#<state>" for the user to copy.
func ManualConfig() FlowConfig {
	return FlowConfig{ClientID: clientID, RedirectURI: redirectURI}
}

// LoopbackConfig is the paste-less flow: the SAME public UUID client as the manual
// flow (the claude.com/cai authorize endpoint validates client_id as a UUID and
// rejects the metadata-document URL client with "Input should be a valid UUID"),
// paired with a http://localhost:<port>/callback the browser redirects to
// automatically. The redirect host MUST be "localhost" (not 127.0.0.1): the
// authorize endpoint normalizes 127.0.0.1 -> localhost, so the token exchange's
// redirect_uri has to match the normalized form or it fails as a redirect mismatch.
// The local listener still binds 127.0.0.1 — browsers resolve localhost to it.
func LoopbackConfig(port int) FlowConfig {
	return FlowConfig{ClientID: clientID, RedirectURI: fmt.Sprintf("http://localhost:%d/callback", port)}
}

// PendingLogin holds the per-attempt PKCE secrets the caller must stash between
// Begin and Exchange (keyed by an opaque flow id). Never sent to the client.
type PendingLogin struct {
	Verifier    string
	State       string
	ClientID    string // the flow's client_id (manual UUID vs loopback metadata URL)
	RedirectURI string // the flow's redirect_uri (platform callback vs localhost)
	CreatedAt   time.Time
}

// Credential is the token set returned by a successful exchange/refresh.
type Credential struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	ExpiresAt    int64    `json:"expiresAt"` // unix MILLISECONDS (Claude Code convention)
	Scopes       []string `json:"scopes"`
	// SubscriptionType is echoed from the token response when present ("max"/"pro");
	// defaults to "max" so the CLI treats it as a subscription credential.
	SubscriptionType string `json:"subscriptionType"`
}

// Begin is the manual (paste-the-code) flow: BeginWith(ManualConfig()).
func Begin() (authURL string, pending PendingLogin, err error) {
	return BeginWith(ManualConfig())
}

// BeginWith creates a fresh PKCE verifier + state for the given flow and returns
// the authorization URL the user should open, plus the PendingLogin to stash until
// Exchange. cfg selects the client_id + redirect_uri (manual vs loopback).
func BeginWith(cfg FlowConfig) (authURL string, pending PendingLogin, err error) {
	verifier, err := randToken(32)
	if err != nil {
		return "", PendingLogin{}, err
	}
	state, err := randToken(24)
	if err != nil {
		return "", PendingLogin{}, err
	}
	challenge := s256Challenge(verifier)
	q := url.Values{}
	q.Set("code", "true") // callback page displays the code (manual) / harmless for loopback
	q.Set("client_id", cfg.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("scope", scopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	authURL = authorizeURL + "?" + q.Encode()
	pending = PendingLogin{
		Verifier:    verifier,
		State:       state,
		ClientID:    cfg.ClientID,
		RedirectURI: cfg.RedirectURI,
		CreatedAt:   time.Now(),
	}
	return authURL, pending, nil
}

// tokenResponse is the token endpoint's JSON reply.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"` // seconds
	Scope            string `json:"scope"`
	SubscriptionType string `json:"subscription_type"`
	// account/organization info may also be present; not needed for credentials.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Exchange trades the pasted authorization code for a credential, verifying the
// PKCE verifier. The pasted value may be "<code>#<state>" (the callback page shows
// them joined) — the state segment, if present, is checked against the pending
// state. Returns an error on any mismatch or a non-2xx token response.
func Exchange(client *http.Client, pending PendingLogin, pastedCode string) (Credential, error) {
	code, state := splitCodeState(pastedCode)
	if state != "" && pending.State != "" && state != pending.State {
		return Credential{}, fmt.Errorf("state mismatch — the pasted code is from a different login attempt; start over")
	}
	if code == "" {
		return Credential{}, fmt.Errorf("empty authorization code")
	}
	// Use the same client_id + redirect_uri the authorization was started with
	// (manual vs loopback); fall back to the manual consts for older pendings.
	cid := pending.ClientID
	if cid == "" {
		cid = clientID
	}
	ruri := pending.RedirectURI
	if ruri == "" {
		ruri = redirectURI
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"state":         pending.State,
		"client_id":     cid,
		"redirect_uri":  ruri,
		"code_verifier": pending.Verifier,
	})
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(string(body)))
	if err != nil {
		return Credential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	_ = json.Unmarshal(raw, &tr)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.AccessToken == "" {
		detail := strings.TrimSpace(tr.Error + " " + tr.ErrorDescription)
		if detail == "" {
			detail = strings.TrimSpace(string(raw))
		}
		return Credential{}, fmt.Errorf("token exchange rejected (HTTP %d): %s", resp.StatusCode, truncate(detail, 300))
	}
	sub := tr.SubscriptionType
	if sub == "" {
		sub = "max"
	}
	scopeList := strings.Fields(tr.Scope)
	if len(scopeList) == 0 {
		scopeList = strings.Fields(scopes)
	}
	return Credential{
		AccessToken:      tr.AccessToken,
		RefreshToken:     tr.RefreshToken,
		ExpiresAt:        time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second).UnixMilli(),
		Scopes:           scopeList,
		SubscriptionType: sub,
	}, nil
}

// credentialsFile is the on-disk shape Claude Code reads from
// <CLAUDE_CONFIG_DIR>/.credentials.json.
type credentialsFile struct {
	ClaudeAiOauth Credential `json:"claudeAiOauth"`
}

// WriteCredentials persists cred as <homeDir>/.credentials.json (0600), creating
// the directory if needed and backing up any existing file to .credentials.json.bak.
// The CLI reads this on its next spawn — no interactive login needed — and refreshes
// it itself using the refresh token.
func WriteCredentials(homeDir string, cred Credential) error {
	if homeDir == "" {
		return fmt.Errorf("empty claude-home dir")
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("create claude-home: %w", err)
	}
	path := filepath.Join(homeDir, ".credentials.json")
	if _, err := os.Stat(path); err == nil {
		_ = os.Rename(path, path+".bak") // best-effort backup of the prior credential
	}
	data, err := json.MarshalIndent(credentialsFile{ClaudeAiOauth: cred}, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically (tmp + rename) so a crash never leaves a half-written file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("finalize credentials: %w", err)
	}
	return nil
}

// --- helpers ---

// randToken returns n random bytes as a base64url (no padding) string — used for
// the PKCE verifier and the state nonce.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// s256Challenge is the PKCE code challenge: base64url(sha256(verifier)), no padding.
func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// splitCodeState splits a pasted "<code>#<state>" (or "<code>&state=<state>", or a
// bare code) into its parts, trimming whitespace the user may have copied.
func splitCodeState(pasted string) (code, state string) {
	pasted = strings.TrimSpace(pasted)
	if i := strings.IndexAny(pasted, "#"); i >= 0 {
		return strings.TrimSpace(pasted[:i]), strings.TrimSpace(pasted[i+1:])
	}
	// tolerate a full callback URL or query fragment being pasted
	if strings.Contains(pasted, "code=") {
		if u, err := url.Parse(pasted); err == nil {
			q := u.Query()
			return q.Get("code"), q.Get("state")
		}
	}
	return pasted, ""
}

func truncate(s string, n int) string { return textutil.TruncBytesEllipsis(s, n) }
