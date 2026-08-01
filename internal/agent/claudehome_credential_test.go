package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCredFile writes a .credentials.json into dir with the given shape.
func writeCredFile(t *testing.T, dir, access, refresh string, expiresAt, refreshExpiresAt int64) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oauth := map[string]any{"accessToken": access, "refreshToken": refresh, "expiresAt": expiresAt}
	if refreshExpiresAt != 0 {
		oauth["refreshTokenExpiresAt"] = refreshExpiresAt
	}
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": oauth})
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// readAccessToken returns the access token stored in dir's credential file.
func readAccessToken(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	if err != nil {
		return ""
	}
	var c oauthCredential
	if json.Unmarshal(b, &c) != nil {
		return ""
	}
	return c.ClaudeAiOauth.AccessToken
}

func msIn(d time.Duration) int64  { return time.Now().Add(d).UnixMilli() }
func msAgo(d time.Duration) int64 { return time.Now().Add(-d).UnixMilli() }

// TestCredentialLiveness covers the classification the seeding choice hangs off.
func TestCredentialLiveness(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, ".credentials.json")

	// The exact shape the CLI leaves behind after a failed OAuth refresh — the file
	// that used to be copied back over and over.
	writeCredFile(t, dir, "", "", 0, 0)
	if credentialLiveness(path).usable {
		t.Error("a wiped credential must not be usable")
	}

	// Access token expired but refresh token present and not known-dead: the CLI
	// refreshes this fine, so it stays usable — just not "live".
	writeCredFile(t, dir, "acc", "ref", msAgo(time.Hour), 0)
	r := credentialLiveness(path)
	if !r.usable {
		t.Error("an expired ACCESS token is refreshable, so it must stay usable")
	}
	if r.liveAccess {
		t.Error("an expired access token is not live")
	}

	// A live access token is the top tier: it proves the home authenticated within
	// the last hour, which is the only thing a dead credential cannot fake.
	writeCredFile(t, dir, "acc", "ref", msIn(time.Hour), 0)
	if r := credentialLiveness(path); !r.usable || !r.liveAccess {
		t.Errorf("a valid access token must rank live, got %+v", r)
	}

	// A recorded refresh-token expiry in the past is authoritative: dead.
	writeCredFile(t, dir, "acc", "ref", msAgo(time.Hour), msAgo(24*time.Hour))
	if credentialLiveness(path).usable {
		t.Error("a credential whose refresh token has expired must not be usable")
	}

	// Missing file.
	if credentialLiveness(filepath.Join(t.TempDir(), ".credentials.json")).usable {
		t.Error("a missing credential must not be usable")
	}
}

// TestCredentialRankPrefersLiveAccessOverFutureRefreshStamp is the exact trap the
// first attempt at this fix fell into: the stale home recorded a
// refreshTokenExpiresAt weeks in the FUTURE, so ranking by expiry stamps alone
// picked it over the user's live login — comparing a refresh-token expiry against
// an access-token expiry. A live access token must win regardless of stamps.
func TestCredentialRankPrefersLiveAccessOverFutureRefreshStamp(t *testing.T) {
	stale, live := t.TempDir(), t.TempDir()
	// Exactly the observed shapes: access expired 19 days ago, refresh stamp 4 days out.
	writeCredFile(t, stale, "STALE", "STALE", msAgo(19*24*time.Hour), msIn(4*24*time.Hour))
	writeCredFile(t, live, "LIVE", "LIVE", msIn(2*time.Hour), 0)

	rs := credentialLiveness(filepath.Join(stale, ".credentials.json"))
	rl := credentialLiveness(filepath.Join(live, ".credentials.json"))
	if !rl.betterThan(rs) {
		t.Errorf("the live login must outrank the stale one; live=%+v stale=%+v", rl, rs)
	}
	if rs.betterThan(rl) {
		t.Error("a future refresh stamp must not beat a live access token")
	}
}

// TestEnsureCredentialPrefersFreshest is the regression guard for the live failure:
// the TionSwarm global home held a credential that had expired weeks earlier, and
// because the heal took the FIRST usable candidate it beat the user's live
// ~/.claude. The CLI then failed to refresh, wiped the workspace credential, and the
// heal copied the same dead file back on every turn.
func TestEnsureCredentialPrefersFreshest(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "global")   // stands in for ~/.tionswarm/claude-home
	live := filepath.Join(root, "userhome")  // stands in for ~/.claude
	dstHome := filepath.Join(root, "wshome") // the workspace claude-home

	writeCredFile(t, stale, "STALE", "STALE", msAgo(19*24*time.Hour), 0)
	writeCredFile(t, live, "LIVE", "LIVE", msIn(2*time.Hour), 0)
	// The workspace home is in the state the CLI leaves after a failed refresh.
	writeCredFile(t, dstHome, "", "", 0, 0)

	pickFreshest(t, dstHome, stale, live)
	if got := readAccessToken(t, dstHome); got != "LIVE" {
		t.Errorf("seeded token = %q, want LIVE (the freshest source, not the first)", got)
	}
}

// TestEnsureCredentialKeepsWorkspaceLogin: a workspace that is logged in and FRESHER
// than any seed source must never be clobbered — a per-workspace login stays in
// control, which is the whole reason each workspace gets its own home.
func TestEnsureCredentialKeepsWorkspaceLogin(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "global")
	live := filepath.Join(root, "userhome")
	dstHome := filepath.Join(root, "wshome")

	writeCredFile(t, stale, "STALE", "STALE", msAgo(19*24*time.Hour), 0)
	writeCredFile(t, live, "LIVE", "LIVE", msIn(2*time.Hour), 0)
	writeCredFile(t, dstHome, "WORKSPACE", "WORKSPACE", msIn(6*time.Hour), 0)

	pickFreshest(t, dstHome, stale, live)
	if got := readAccessToken(t, dstHome); got != "WORKSPACE" {
		t.Errorf("seeded token = %q, want the workspace's own (freshest) login", got)
	}
}

// TestEnsureCredentialSeedsWhenNothingFresher: with a wiped workspace home and only
// a stale source available, seeding the stale one still beats leaving the home
// unusable — an old credential may still refresh; an empty one never can.
func TestEnsureCredentialSeedsWhenNothingFresher(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "global")
	dstHome := filepath.Join(root, "wshome")

	writeCredFile(t, stale, "STALE", "STALE", msAgo(19*24*time.Hour), 0)
	writeCredFile(t, dstHome, "", "", 0, 0)

	pickFreshest(t, dstHome, stale)
	if got := readAccessToken(t, dstHome); got != "STALE" {
		t.Errorf("seeded token = %q, want STALE (better than an empty home)", got)
	}
}

// pickFreshest exercises the same selection ensureClaudeHomeCredential performs,
// with explicit candidate dirs. ensureClaudeHomeCredential itself reads the real
// ~/.tionswarm and ~/.claude, which a test must not depend on.
func pickFreshest(t *testing.T, dstHome string, candidateDirs ...string) {
	t.Helper()
	dst := filepath.Join(dstHome, ".credentials.json")
	best := credentialLiveness(dst)
	chosen := ""
	for _, dir := range candidateDirs {
		src := filepath.Join(dir, ".credentials.json")
		if r := credentialLiveness(src); r.betterThan(best) {
			chosen, best = src, r
		}
	}
	if chosen != "" {
		if err := copyFile(chosen, dst, 0o600); err != nil {
			t.Fatalf("copy: %v", err)
		}
	}
}
