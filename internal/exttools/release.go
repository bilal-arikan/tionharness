package exttools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/config"
)

// releaseTTL is how long a fetched release stays fresh. GitHub's unauthenticated
// API allows 60 requests/hour per IP and TionSwarm has no token: with seven tools
// a user clicking "check" a few times would exhaust it in minutes. Six hours is
// far shorter than any tool's release cadence and keeps the budget untouched.
const releaseTTL = 6 * time.Hour

// releaseHTTPTimeout caps one upstream call so a slow network cannot stall the
// whole check (which fans out over every tool).
const releaseHTTPTimeout = 10 * time.Second

// Release is the latest published version of one tool, as cached on disk.
type Release struct {
	Repo        string    `json:"repo"`
	Tag         string    `json:"tag"`
	URL         string    `json:"url"`
	PublishedAt string    `json:"publishedAt,omitempty"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

// Fresh reports whether this cache entry is still within the TTL.
func (r Release) Fresh(now time.Time) bool { return now.Sub(r.FetchedAt) < releaseTTL }

// releaseCache guards the in-memory copy of the on-disk cache file. The cache is
// process-wide (not per-workspace): a tool's published version is a property of
// the internet, not of a workspace.
var releaseCache = struct {
	sync.Mutex
	loaded  bool
	entries map[string]Release // keyed by "owner/repo"
}{entries: map[string]Release{}}

// cachePath is the on-disk cache location, under the same data dir as the logs.
func cachePath() string {
	return filepath.Join(config.DefaultDataDir(), "cache", "exttools-releases.json")
}

// loadCacheLocked reads the cache file once per process. A missing or corrupt
// file is not an error — it just means an empty cache and a refetch.
func loadCacheLocked() {
	if releaseCache.loaded {
		return
	}
	releaseCache.loaded = true
	raw, err := os.ReadFile(cachePath())
	if err != nil {
		return
	}
	var entries map[string]Release
	if json.Unmarshal(raw, &entries) == nil && entries != nil {
		releaseCache.entries = entries
	}
}

// saveCacheLocked persists the cache atomically (tmp + rename), matching the
// write-through pattern the rest of TionSwarm's file storage uses. A write
// failure is returned so the caller can log it; the in-memory cache still holds.
func saveCacheLocked() error {
	p := cachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(releaseCache.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// githubAPI is the release endpoint base, overridable in tests.
var githubAPI = "https://api.github.com"

// LatestRelease returns the newest published release for a GitHub "owner/repo".
//
// Cache-first: a fresh entry is returned without any network call. On a network
// or rate-limit failure it FAILS OPEN — a stale cached entry is returned with
// stale=true rather than an error, because a missed update check must never make
// the external-tools screen look broken. Only a miss with no cache at all errors.
func LatestRelease(ctx context.Context, repo string) (rel Release, stale bool, err error) {
	if repo == "" {
		return Release{}, false, fmt.Errorf("bu araç için GitHub release akışı yok")
	}

	releaseCache.Lock()
	loadCacheLocked()
	cached, hit := releaseCache.entries[repo]
	releaseCache.Unlock()

	if hit && cached.Fresh(time.Now()) {
		return cached, false, nil
	}

	fetched, fetchErr := fetchLatest(ctx, repo)
	if fetchErr != nil {
		if hit {
			return cached, true, nil // stale but usable
		}
		return Release{}, false, fetchErr
	}

	releaseCache.Lock()
	releaseCache.entries[repo] = fetched
	// The fetched value is correct regardless of whether it persists; a failed
	// write only costs a refetch next process. Recorded (under the same lock) so
	// a broken data dir shows up in the API layer's log instead of vanishing.
	cacheWriteErr = saveCacheLocked()
	releaseCache.Unlock()
	return fetched, false, nil
}

// cacheWriteErr holds the last release-cache persist failure. Guarded by
// releaseCache's mutex; read via CacheWriteErr.
var cacheWriteErr error

// CacheWriteErr returns the last release-cache persist failure, or nil.
func CacheWriteErr() error {
	releaseCache.Lock()
	defer releaseCache.Unlock()
	return cacheWriteErr
}

// InvalidateCache drops every cached release so the next check refetches. Backs
// an explicit "force refresh" from the UI.
func InvalidateCache() {
	releaseCache.Lock()
	defer releaseCache.Unlock()
	loadCacheLocked()
	releaseCache.entries = map[string]Release{}
	_ = saveCacheLocked()
}

// fetchLatest calls GitHub's releases/latest endpoint. Note this endpoint skips
// prereleases and drafts by design, which is what a user wants when asked "is
// there a newer stable version".
func fetchLatest(ctx context.Context, repo string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, releaseHTTPTimeout)
	defer cancel()

	url := githubAPI + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TionSwarm")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("GitHub'a ulaşılamadı: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, fmt.Errorf("%s için yayımlanmış release yok", repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Release{}, fmt.Errorf("GitHub API limiti aşıldı (saatte 60 istek) — daha sonra tekrar dene")
	default:
		return Release{}, fmt.Errorf("GitHub %s döndü", resp.Status)
	}

	var body struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("GitHub yanıtı okunamadı: %w", err)
	}
	if body.TagName == "" {
		return Release{}, fmt.Errorf("GitHub yanıtında sürüm etiketi yok")
	}
	return Release{
		Repo:        repo,
		Tag:         body.TagName,
		URL:         body.HTMLURL,
		PublishedAt: body.PublishedAt,
		FetchedAt:   time.Now(),
	}, nil
}
