package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/exttools"
)

// The codex-cli provider is the second keyless CLI transport (the sibling of
// claude-cli): its models are OpenAI slugs but what actually runs them is a
// locally installed Codex CLI on a ChatGPT/Codex subscription. The catalog
// carries the installed binary's version next to the model list, mirroring
// claudeCLIVersion below.
const (
	// codexCLIVersionTTL bounds how long a probed `codex --version` is reused —
	// same rationale as claudeCLIVersionTTL: memoise across catalog loads, refresh
	// periodically since the CLI can self-update in the background.
	codexCLIVersionTTL = 10 * time.Minute
	// codexCLIVersionFailTTL is the shorter reuse window for a failed probe, so a
	// freshly-fixed binary shows up quickly instead of staying invisible for the
	// full TTL.
	codexCLIVersionFailTTL = time.Minute
)

var codexVerCache struct {
	mu    sync.Mutex
	path  string
	value string
	at    time.Time
}

// codexCLIVersion returns the installed Codex CLI version for the binary at
// path, or "" when it cannot be read.
//
// An unreadable version is not worth failing the catalog over: the picker
// simply omits the badge and every other provider entry still renders.
func codexCLIVersion(ctx context.Context, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	codexVerCache.mu.Lock()
	defer codexVerCache.mu.Unlock()

	ttl := codexCLIVersionTTL
	if codexVerCache.value == "" {
		ttl = codexCLIVersionFailTTL
	}
	if codexVerCache.path == path && time.Since(codexVerCache.at) < ttl {
		return codexVerCache.value
	}

	version, err := exttools.LocalVersion(ctx, path, []string{"--version"})
	if err != nil {
		version = ""
	}
	codexVerCache.path, codexVerCache.value, codexVerCache.at = path, version, time.Now()
	return version
}

// codexAuthFile is the credential file codex-cli writes inside CODEX_HOME on a
// successful `codex login` (see _Docs/69 §4). Only its presence is checked here
// — the JSON shape is Codex's own and not part of TionSwarm's contract, so this
// intentionally reads no field beyond "does the file exist and hold something".
const codexAuthFile = "auth.json"

// codexSubscriptionTier reports "chatgpt" when a codex-home holds a readable
// auth.json, or "" when there is none or it cannot be read. Codex's auth.json
// (unlike Claude Code's) does not expose a plan tier ("max"/"pro") in a stable
// field, so this can only report login presence, not a plan name — callers must
// not assume parity with claudeSubscriptionTier's richer output.
func codexSubscriptionTier(homeDir string) string {
	if homeDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(homeDir, codexAuthFile))
	if err != nil || len(data) == 0 {
		return ""
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil || len(probe) == 0 {
		return ""
	}
	return "chatgpt"
}
