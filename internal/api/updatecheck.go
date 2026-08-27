package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// In-app update check.
//
// The app asks the release feed (<FeedURL()>/latest.json) whether a newer
// version exists and surfaces the answer as a dismissible banner. Nothing is
// downloaded and nothing self-updates: the banner only links to the release
// notes and to the download for this platform, so the user stays in control of
// how the binary is replaced.
//
// Two rules shape the behaviour:
//
//   - An UNSTAMPED build (BuildVersion == "dev") never checks. A developer
//     running from source has no meaningful "current version" to compare, and
//     nagging them to upgrade to a release they are ahead of is pure noise.
//   - A feed that is unreachable, non-200 or malformed yields "unknown", logged
//     and otherwise silent. This is the one place a swallowed error is right:
//     the check is advisory, it runs unattended at startup, and a network blip
//     must not produce an error banner or hold anything up.

const (
	// updateCheckTTL is the minimum spacing between two real fetches. The feed
	// is a static file, and a release the user hears about a few hours late
	// costs nothing.
	updateCheckTTL = 24 * time.Hour
	// updateCheckTimeout caps the whole request. Short on purpose: this runs in
	// the background and nobody is waiting on it.
	updateCheckTimeout = 8 * time.Second
	// updateCheckStartupDelay keeps the check out of the way of boot, which is
	// already doing workspace loading and provider wiring.
	updateCheckStartupDelay = 15 * time.Second
	// devVersion is the BuildVersion of a build made without -ldflags.
	devVersion = "dev"
)

// updateFeed is the subset of latest.json the check needs. Only Version is
// required; released_at, notes_url and artifacts are read when present and left
// empty otherwise, since the feed does not guarantee them.
type updateFeed struct {
	Version    string           `json:"version"`
	ReleasedAt string           `json:"released_at"`
	NotesURL   string           `json:"notes_url"`
	Artifacts  []updateArtifact `json:"artifacts"`
}

// updateStatus is the API shape. State separates the three real outcomes so the
// UI never has to infer "we could not reach the feed" from a blank latest:
//
//	"ok"      — the feed was read; Available says whether to upgrade
//	"skipped" — unstamped dev build, no check was made
//	"unknown" — the feed could not be read
//
// DownloadURL is either the artifact built for this server's platform or, when
// the feed lists none for it, the generic releases page. DownloadFile is the
// artifact's file name and is empty in the fallback case, which is how the UI
// tells "direct download" from "go pick one yourself".
type updateStatus struct {
	State        string `json:"state"`
	Current      string `json:"current"`
	Latest       string `json:"latest"`
	Available    bool   `json:"updateAvailable"`
	NotesURL     string `json:"notesUrl"`
	ReleasedAt   string `json:"releasedAt"`
	CheckedAt    string `json:"checkedAt"`
	DownloadURL  string `json:"downloadUrl"`
	DownloadFile string `json:"downloadFile"`
}

// updateChecker owns the cached result. The mutex is held across the fetch on
// purpose: concurrent callers then collapse onto a single request instead of
// each opening their own, and the losers return the fresh cache the winner just
// wrote.
type updateChecker struct {
	mu       sync.Mutex
	client   *http.Client
	now      func() time.Time
	ttl      time.Duration
	cached   updateStatus
	cachedAt time.Time
	hasCache bool
}

func newUpdateChecker() *updateChecker {
	return &updateChecker{
		client: &http.Client{Timeout: updateCheckTimeout},
		now:    time.Now,
		ttl:    updateCheckTTL,
	}
}

// updates returns the server's checker, creating it on first use so a bare
// Server{} (as tests construct) works without extra wiring.
func (s *Server) updates() *updateChecker {
	s.updatesOnce.Do(func() {
		if s.updateChk == nil {
			s.updateChk = newUpdateChecker()
		}
	})
	return s.updateChk
}

// Status returns the cached result, refreshing it when the cache is empty or
// older than the TTL.
func (c *updateChecker) Status(ctx context.Context, s *Server) updateStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	if BuildVersion == devVersion {
		// Never cached: a test (or a future stamped restart) must see the switch
		// away from "dev" immediately, and skipping costs nothing anyway.
		return updateStatus{State: "skipped", Current: BuildVersion}
	}
	if c.hasCache && c.now().Sub(c.cachedAt) < c.ttl {
		return c.cached
	}

	status, err := c.fetch(ctx)
	if err != nil {
		// Advisory check: log and report "unknown" rather than surfacing an error.
		if s != nil && s.logger != nil {
			s.logger.Info("update check failed", "url", FeedURL()+"/latest.json", "err", err)
		}
		status = updateStatus{State: "unknown", Current: BuildVersion}
	}
	status.CheckedAt = c.now().UTC().Format(time.RFC3339)
	c.cached = status
	c.cachedAt = c.now()
	c.hasCache = true
	return status
}

func (c *updateChecker) fetch(ctx context.Context) (updateStatus, error) {
	url := FeedURL() + "/latest.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return updateStatus{}, err
	}
	req.Header.Set("User-Agent", "TionHarness/"+BuildVersion+" (update-check)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return updateStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return updateStatus{}, fmt.Errorf("feed returned %s", resp.Status)
	}
	// Cap the read: a feed that answers with a huge body is broken, and this
	// runs unattended.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return updateStatus{}, err
	}
	var feed updateFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		return updateStatus{}, fmt.Errorf("decode feed: %w", err)
	}
	if feed.Version == "" {
		return updateStatus{}, fmt.Errorf("feed has no version")
	}
	newer, err := versionIsNewer(feed.Version, BuildVersion)
	if err != nil {
		return updateStatus{}, err
	}
	// Fall back to the releases page: an artifact for another platform is worse
	// than no direct link at all.
	download, file := releasesURL(), ""
	if art, ok := selectArtifact(feed.Artifacts, runtime.GOOS, runtime.GOARCH); ok {
		download, file = art.URL, art.File
	}
	return updateStatus{
		State:        "ok",
		Current:      BuildVersion,
		Latest:       feed.Version,
		Available:    newer,
		NotesURL:     feed.NotesURL,
		ReleasedAt:   feed.ReleasedAt,
		DownloadURL:  download,
		DownloadFile: file,
	}, nil
}

// StartUpdateCheck runs one check shortly after boot, in the background. The
// result lands in the cache, so the UI's first request is answered without
// waiting on the network. Called once from app bootstrap.
func (s *Server) StartUpdateCheck() {
	go func() {
		time.Sleep(updateCheckStartupDelay)
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		s.updates().Status(ctx, s)
	}()
}

// handleUpdateCheck reports whether a newer release exists.
// GET /api/version/update
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.updates().Status(r.Context(), s))
}
