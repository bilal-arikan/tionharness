package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/claudeauth"
	"github.com/bilal-arikan/tionswarm/internal/exttools"
)

// The claude-cli provider is the odd one out in the catalog: its models are bare
// aliases ("sonnet", "opus") and what actually runs them is a locally installed
// Claude Code binary on a Max/Pro subscription. Two facts about that install —
// its version and the plan it is logged into — are what distinguish one machine's
// "sonnet" from another's, so the catalog carries them next to the model list.

const (
	// claudeCLIVersionTTL bounds how long a probed `claude --version` is reused.
	// The catalog is fetched on page load and the probe spawns a subprocess, so
	// the answer is memoised; Claude Code self-updates in the background, hence a
	// TTL rather than a process-lifetime memo.
	claudeCLIVersionTTL = 10 * time.Minute
	// claudeCLIVersionFailTTL is the (much shorter) reuse window for a failed
	// probe. Failures are cached too — otherwise a broken binary costs the 3s
	// probe timeout on every catalog load — but only briefly, so a fixed install
	// shows up quickly instead of staying invisible for the full TTL.
	claudeCLIVersionFailTTL = time.Minute
)

var claudeVerCache struct {
	mu    sync.Mutex
	path  string
	value string
	at    time.Time
}

// claudeCLIVersion returns the installed Claude Code version ("2.1.220") for the
// binary at path, or "" when it cannot be read.
//
// An unreadable version is not worth failing the catalog over: the picker simply
// omits the badge and every other provider entry still renders.
func claudeCLIVersion(ctx context.Context, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	claudeVerCache.mu.Lock()
	defer claudeVerCache.mu.Unlock()

	ttl := claudeCLIVersionTTL
	if claudeVerCache.value == "" {
		ttl = claudeCLIVersionFailTTL
	}
	if claudeVerCache.path == path && time.Since(claudeVerCache.at) < ttl {
		return claudeVerCache.value
	}

	version, err := exttools.LocalVersion(ctx, path, []string{"--version"})
	if err != nil {
		version = ""
	}
	claudeVerCache.path, claudeVerCache.value, claudeVerCache.at = path, version, time.Now()
	return version
}

// claudeSubscriptionTier reports the Claude Code plan backing a claude-home
// ("max" / "pro"), or "" when the credential is not a subscription or the tier
// cannot be determined.
//
// authKind is the app-level credential override: "apikey" means the CLI runs on a
// per-token API key rather than a plan, so no tier is reported even if a stale
// OAuth credential is still lying around in the home.
func claudeSubscriptionTier(homeDir, authKind string) string {
	if authKind == "apikey" {
		return ""
	}
	cred, err := claudeauth.ReadCredentials(homeDir)
	if err != nil {
		return "" // never logged in here, or the home holds no readable credential
	}
	return strings.ToLower(strings.TrimSpace(cred.SubscriptionType))
}
