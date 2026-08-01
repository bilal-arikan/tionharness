package agent

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// workspaceClaudeHomeDir is a workspace's per-workspace claude-cli config home
// (<workspace>/claude-home), a sibling of store/, config/ and workspace/. It is
// exported into the CLI subprocess as CLAUDE_CONFIG_DIR so every workspace runs the
// CLI against its OWN skills/settings/login instead of one global home. workDir is
// the sandbox root (<workspace>/workspace); the home is its sibling. Empty when
// workDir is unknown.
func workspaceClaudeHomeDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "claude-home")
}

// globalClaudeHomeDir mirrors settings.defaultClaudeConfigDir: the TionSwarm-managed
// global claude-cli config home (~/.tionswarm/claude-home) that holds the shared
// login/settings before per-workspace homes existed. It is the seed source copied
// into a fresh per-workspace home so the workspace CLI starts already authenticated.
// Empty when the user home cannot be resolved (no seed → the CLI relies on the
// injected auth env instead).
func globalClaudeHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionswarm", "claude-home")
}

// claudeHomeEphemeralDirs are per-run/history subdirectories NOT copied when seeding
// a per-workspace claude-home from the global one: they are large and run-specific
// (session transcripts, shell snapshots), not config the workspace should inherit.
var claudeHomeEphemeralDirs = map[string]bool{
	"projects":        true, // per-project session transcripts
	"sessions":        true, // CLI session storage
	"session-env":     true,
	"file-history":    true, // edit history snapshots
	"shell-snapshots": true,
	"todos":           true,
	"tasks":           true, // per-run scheduled tasks (avoid duplicating across workspaces)
	"cache":           true, // large, regenerable
	"backups":         true,
	"statsig":         true,
	"context-mode":    true,
	"logs":            true,
}

// EnsureWorkspaceClaudeHome provisions the per-workspace claude-cli config home for a
// workspace root (<workspace>). It is idempotent and safe to call on every workspace
// open: if <workspace>/claude-home does not exist, it creates it and seeds it from the
// global ~/.tionswarm/claude-home (config + login, skipping per-run/history dirs) so
// the workspace CLI starts already logged in.
//
// Skills are NOT stored here: the workspace skill tier stays at <workspace>/skills
// (served by TionSwarm's use_skill bridge; the CLI's native Skill tool is disabled —
// see climcp.go). claude-home holds only the CLI's login/settings.
//
// Filesystem errors are best-effort: a failed seed leaves the workspace usable (auth
// still flows via the injected env) rather than blocking workspace open.
func EnsureWorkspaceClaudeHome(wsRoot string) {
	if wsRoot == "" {
		return
	}
	home := filepath.Join(wsRoot, "claude-home")

	if _, err := os.Stat(home); os.IsNotExist(err) {
		_ = os.MkdirAll(home, 0o755)
		if src := globalClaudeHomeDir(); src != "" && src != home {
			if fi, statErr := os.Stat(src); statErr == nil && fi.IsDir() {
				_ = copyClaudeHome(src, home)
			}
		}
	}
	// Runs on EVERY workspace open (not only fresh seeds) so existing homes are
	// healed too — the effortLevel guard matters for all CLI turns, including the
	// non-MCP path that gets no per-turn --settings file.
	ensureClaudeHomeEffortLevel(home)
	// Same "heal on every open" reasoning for the login credential: a home seeded
	// from an unauthenticated global home (blank-token .credentials.json) would
	// otherwise pop a login prompt on the first CLI turn.
	ensureClaudeHomeCredential(home)
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
// take the FIRST usable candidate, so a TionSwarm global home whose access token
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
// best-ranked usable credential is copied in from the TionSwarm global home
// (~/.tionswarm/claude-home) or the user's real ~/.claude. Falling back to ~/.claude
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
	if best != "" {
		_ = copyFile(best, dst, 0o600)
	}
}

// ensureClaudeHomeEffortLevel makes sure a workspace claude-home's settings.json
// carries an EXPLICIT effortLevel. Claude Code ≥2.1.203 serialises parallel tool
// calls when effortLevel is unset (default adaptive effort) — each Write becomes
// its own API call, multiplying prompt-cache re-reads and cost (the AlgoBench
// v2/v3 regression: workspace homes seeded minimal settings while the user's own
// ~/.claude carried "high", so only TionSwarm turns degraded). The value is
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

// copyClaudeHome recursively copies the config-relevant contents of the global
// claude-home into a fresh per-workspace home, skipping the ephemeral per-run/history
// dirs (claudeHomeEphemeralDirs). Only top-level ephemeral dirs are skipped; nested
// content is copied verbatim.
func copyClaudeHome(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && claudeHomeEphemeralDirs[name] {
			continue
		}
		if err := copyPath(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

// copyPath copies a single file or directory tree from src to dst, preserving the
// source's file mode. Symlinks are followed as regular files.
func copyPath(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst, fi.Mode())
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
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
