package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/exttools"
)

// The opencode-cli provider is the third CLI transport (anomalyco/opencode).
// Its credentials are opencode's own: `opencode auth login` stores them in the
// home's data dir (<home>/data/opencode/auth.json, keyed by provider id). The
// catalog shows the installed binary's version and which vendors hold a
// credential, mirroring the codex helpers.

var opencodeVerCache struct {
	mu    sync.Mutex
	path  string
	value string
	at    time.Time
}

// opencodeCLIVersion returns the installed opencode version for the binary at
// path, or "" when it cannot be read (memoised like codexCLIVersion).
func opencodeCLIVersion(ctx context.Context, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	opencodeVerCache.mu.Lock()
	defer opencodeVerCache.mu.Unlock()
	ttl := codexCLIVersionTTL
	if opencodeVerCache.value == "" {
		ttl = codexCLIVersionFailTTL
	}
	if opencodeVerCache.path == path && time.Since(opencodeVerCache.at) < ttl {
		return opencodeVerCache.value
	}
	version, err := exttools.LocalVersion(ctx, path, []string{"--version"})
	if err != nil {
		version = ""
	}
	opencodeVerCache.path, opencodeVerCache.value, opencodeVerCache.at = path, version, time.Now()
	return version
}

// opencodeAuthFile is where opencode keeps its credentials relative to the
// home TionHarness exports as XDG_DATA_HOME (<home>/data).
func opencodeAuthFile(homeDir string) string {
	return filepath.Join(homeDir, "data", "opencode", "auth.json")
}

// opencodeAuthProviders lists the vendor ids that hold a credential in the
// home's auth store, sorted. Only key presence is read; the entry shapes are
// opencode's own. nil when there is no store or it is empty.
func opencodeAuthProviders(homeDir string) []string {
	if homeDir == "" {
		return nil
	}
	data, err := os.ReadFile(opencodeAuthFile(homeDir))
	if err != nil || len(data) == 0 {
		return nil
	}
	var entries map[string]json.RawMessage
	if json.Unmarshal(data, &entries) != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for k, v := range entries {
		if strings.TrimSpace(string(v)) != "" && string(v) != "null" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// opencodeAuthSummary renders the credential holders as the catalog's
// subscription badge ("anthropic, openai"); "" when none.
func opencodeAuthSummary(homeDir string) string {
	return strings.Join(opencodeAuthProviders(homeDir), ", ")
}

// opencodeAuthStatus reports install + credential state for the provider auth
// panel. Unlike codex no live probe runs: opencode's free Zen models need no
// credential at all, so "no stored credential" is informational, not a failure.
func opencodeAuthStatus(homeDir, binPath string) providerAuthDTO {
	dto := providerAuthDTO{Kind: "opencode-cli", HomeDir: homeDir, Installed: strings.TrimSpace(binPath) != ""}
	if !dto.Installed {
		dto.Detail = "opencode CLI not found (install: npm i -g opencode-ai, or set its path in Providers)"
		return dto
	}
	vendors := opencodeAuthProviders(homeDir)
	if len(vendors) == 0 {
		dto.Detail = "no provider credential stored in this home; run `opencode auth login` with XDG_DATA_HOME=" + filepath.Join(homeDir, "data") + " (free opencode/* models work without one)"
		return dto
	}
	dto.LoggedIn = true
	dto.Tier = strings.Join(vendors, ", ")
	return dto
}
