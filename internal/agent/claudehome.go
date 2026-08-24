package agent

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// globalClaudeHomeDir mirrors settings.defaultClaudeConfigDir: the TionHarness-managed
// global claude-cli config home (~/.tionharness/claude-home) that holds the shared
// login/settings before per-workspace homes existed. It is the seed source copied
// into a fresh per-workspace home so the workspace CLI starts already authenticated.
// Empty when the user home cannot be resolved (no seed → the CLI relies on the
// injected auth env instead).
func globalClaudeHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionharness", "claude-home")
}

// oauthCredential is the token-bearing part of the on-disk .credentials.json Claude
// Code reads/writes (<home>/.credentials.json → {"claudeAiOauth": {...}}).
type oauthCredential struct {
	ClaudeAiOauth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		// ExpiresAt is the ACCESS token's expiry (unix ms). An expired access token
		// is normal — the CLI refreshes it — but a long-expired one is the only
		// on-disk hint that the whole credential may be dead.
		ExpiresAt int64 `json:"expiresAt"`
		// RefreshTokenExpiresAt is written by newer CLI builds (unix ms; absent on
		// older files). When present it is the authoritative "this credential can no
		// longer authenticate after" stamp.
		RefreshTokenExpiresAt int64 `json:"refreshTokenExpiresAt"`
	} `json:"claudeAiOauth"`
}

// credentialUsable reports whether the .credentials.json at path carries a token the
// CLI can actually use — a non-empty access OR refresh token (the CLI self-refreshes
// from the refresh token, so a live access token is not required). A missing, empty,
// or blank-token file is NOT usable: seeding it into a workspace yields an interactive
// login prompt on the first CLI turn. This is exactly the empty-scaffold shape
// (accessToken:"", refreshToken:"", expiresAt:0) that made fresh workspaces prompt.
//
// It also rejects a credential whose refresh token is KNOWN to have expired
// (refreshTokenExpiresAt in the past): copying that into a workspace produces the
// same login prompt, one CLI turn later.
func credentialUsable(path string) bool {
	return credentialLiveness(path).usable
}

// credentialRank orders candidate credential files for seeding.
type credentialRank struct {
	// usable is false for a missing/unparseable/wiped file, or one whose refresh
	// token is recorded as already expired. Seeding such a file yields a login
	// prompt on the first CLI turn.
	usable bool
	// liveAccess is true when the ACCESS token has not expired yet — proof that
	// this home authenticated successfully very recently. It is the primary
	// ordering key, ahead of any expiry stamp, for the reason in credentialLiveness.
	liveAccess bool
	// expiresAt is the access-token expiry (unix ms): freshness within a tier.
	expiresAt int64
}

// betterThan reports whether r is a better seed source than o.
func (r credentialRank) betterThan(o credentialRank) bool {
	if r.usable != o.usable {
		return r.usable
	}
	if r.liveAccess != o.liveAccess {
		return r.liveAccess
	}
	return r.expiresAt > o.expiresAt
}

// credentialLiveness ranks a credential file as a seed source.
//
// Ordering by "a live access token first" is not arbitrary — it is the only signal
// on disk that actually predicted the live failure this fixes. The heal used to
// take the FIRST usable candidate, so a TionHarness global home whose access token
// had expired 19 days earlier beat the user's live ~/.claude. Its refresh token had
// long since been consumed (they are single-use), so the CLI got invalid_grant,
// CLEARED the workspace credential, and the heal copied the same dead file back on
// every turn.
//
// Ranking by expiry stamps alone does NOT fix that: the stale home recorded a
// refreshTokenExpiresAt weeks in the FUTURE — it looks alive on disk and is not.
// Whether a refresh token has already been spent is simply not observable here. A
// non-expired ACCESS token is, and it means the home authenticated within the last
// hour, which no dead credential can fake.
func credentialLiveness(path string) credentialRank {
	b, err := os.ReadFile(path)
	if err != nil {
		return credentialRank{}
	}
	var c oauthCredential
	if json.Unmarshal(b, &c) != nil {
		return credentialRank{}
	}
	o := c.ClaudeAiOauth
	if o.AccessToken == "" && o.RefreshToken == "" {
		return credentialRank{} // wiped scaffold or never logged in
	}
	nowMs := time.Now().UnixMilli()
	if o.RefreshTokenExpiresAt > 0 && o.RefreshTokenExpiresAt <= nowMs {
		return credentialRank{} // authoritatively dead: cannot authenticate at all
	}
	return credentialRank{
		usable:     true,
		liveAccess: o.AccessToken != "" && o.ExpiresAt > nowMs,
		expiresAt:  o.ExpiresAt,
	}
}

// ensureClaudeHomeCredential makes sure a workspace claude-home is logged in. The
// home's OWN credential wins whenever nothing available ranks better — a
// per-workspace login stays in control and is never clobbered. Otherwise the
// best-ranked usable credential is copied in from the TionHarness global home
// (~/.tionharness/claude-home) or the user's real ~/.claude. Falling back to ~/.claude
// matches the keyless claude-cli design (it runs against the user's local login).
//
// Ranked, not first-match: see credentialLiveness for why fixed candidate order was
// the bug. Called both on workspace open and at the per-turn CLI seam, so a home the
// CLI wiped mid-run recovers on the next turn instead of failing until a restart.
//
// Best-effort: if no usable source exists, the home is left as-is and the user logs in
// once via the in-app popup (which writes this workspace's home directly).
func ensureClaudeHomeCredential(home string) {
	dst := filepath.Join(home, ".credentials.json")

	// Serialize heals per home. Concurrent CLI turns all reach this seam, and
	// without the lock several would evaluate the same wiped destination and race
	// to reseed it. The copy itself is atomic, so this is not about a torn file —
	// it is about the decision: between ranking and copying, the CLI can refresh
	// dst with a NEWER token, and an unsynchronized heal would then overwrite a
	// live login with an older source.
	unlock := lockCredentialHeal(home)
	defer unlock()

	var candidates []string
	if g := globalClaudeHomeDir(); g != "" && g != home {
		candidates = append(candidates, filepath.Join(g, ".credentials.json"))
	}
	if uh, err := os.UserHomeDir(); err == nil && uh != "" {
		candidates = append(candidates, filepath.Join(uh, ".claude", ".credentials.json"))
	}

	best, bestRank := "", credentialLiveness(dst)
	for _, src := range candidates {
		if r := credentialLiveness(src); r.betterThan(bestRank) {
			best, bestRank = src, r
		}
	}
	if best == "" {
		return
	}
	// Re-read the destination immediately before overwriting it: the ranking above
	// is only as fresh as the moment it ran, and a CLI refresh landing in that
	// window must win over anything we were about to seed.
	if credentialLiveness(dst).betterThan(bestRank) {
		return
	}
	_ = copyFile(best, dst, 0o600)
}

// credentialHealLocks holds one mutex per claude-home, keyed by path.
var credentialHealLocks sync.Map

// lockCredentialHeal serializes credential healing for one claude-home and returns
// the unlock func.
func lockCredentialHeal(home string) func() {
	v, _ := credentialHealLocks.LoadOrStore(home, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ensureClaudeHomeEffortLevel makes sure a workspace claude-home's settings.json
// carries an EXPLICIT effortLevel. Claude Code ≥2.1.203 serialises parallel tool
// calls when effortLevel is unset (default adaptive effort) — each Write becomes
// its own API call, multiplying prompt-cache re-reads and cost (the AlgoBench
// v2/v3 regression: workspace homes seeded minimal settings while the user's own
// ~/.claude carried "high", so only TionHarness turns degraded). The value is
// copied ONCE from the user's real ~/.claude/settings.json when set there, else
// defaults to "high" (pre-2.1.203 batching parity). An existing explicit
// effortLevel is never overwritten, so the user stays in control per workspace.
// Best-effort: an unreadable/unparseable settings file is left untouched.
func ensureClaudeHomeEffortLevel(home string) {
	path := filepath.Join(home, "settings.json")
	cfg := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &cfg) != nil {
			return // unparseable: do not risk clobbering a hand-edited file
		}
	}
	if _, ok := cfg["effortLevel"]; ok {
		return
	}
	effort := "high"
	if uh, err := os.UserHomeDir(); err == nil && uh != "" {
		if b, err := os.ReadFile(filepath.Join(uh, ".claude", "settings.json")); err == nil {
			var user map[string]any
			if json.Unmarshal(b, &user) == nil {
				if v, ok := user["effortLevel"].(string); ok && v != "" {
					effort = v
				}
			}
		}
	}
	cfg["effortLevel"] = effort
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

// copyFile copies a regular file's contents from src to dst with the given mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Write to a temp file and rename into place. The destination may be read
	// CONCURRENTLY by a claude CLI subprocess — .credentials.json especially, now
	// that the credential heal runs at the per-turn seam — and truncate-then-stream
	// would expose a zero-length or half-written file to whoever reads it in
	// between, which the CLI reports as "not logged in". Rename is atomic within a
	// directory on both POSIX and Windows (MOVEFILE_REPLACE_EXISTING).
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-copy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeded
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
