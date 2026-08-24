package exttools

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// resetCacheForTest points the on-disk cache at a temp data dir and clears the
// in-memory copy, so each test starts from a cold cache without touching the
// developer's real ~/.tionharness.
func resetCacheForTest(t *testing.T) {
	t.Helper()
	t.Setenv("TIONHARNESS_DATA_DIR", t.TempDir())
	releaseCache.Lock()
	releaseCache.loaded = true // skip loading the (empty) temp file
	releaseCache.entries = map[string]Release{}
	cacheWriteErr = nil
	releaseCache.Unlock()
}

// serveLatest stands in for GitHub, counting how many times it was called so the
// cache can be proven to actually prevent requests.
func serveLatest(t *testing.T, status int, tag string, hits *int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hits++
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example.test/rel","published_at":"2026-07-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	prev := githubAPI
	githubAPI = srv.URL
	t.Cleanup(func() { githubAPI = prev })
}

// A fresh cache entry must be served without a second request. This is the whole
// point of the cache: GitHub allows 60 unauthenticated calls/hour and the panel
// checks seven tools at a time.
func TestLatestReleaseCachesWithinTTL(t *testing.T) {
	resetCacheForTest(t)
	hits := 0
	serveLatest(t, http.StatusOK, "v1.2.3", &hits)

	for i := 0; i < 3; i++ {
		rel, stale, err := LatestRelease(t.Context(), "owner/repo")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if stale {
			t.Fatalf("call %d: fresh fetch reported stale", i)
		}
		if rel.Tag != "v1.2.3" {
			t.Fatalf("call %d: tag = %q", i, rel.Tag)
		}
	}
	if hits != 1 {
		t.Fatalf("upstream hit %d times, want 1 (cache did not hold)", hits)
	}
}

func TestLatestReleaseRefetchesAfterTTL(t *testing.T) {
	resetCacheForTest(t)
	hits := 0
	serveLatest(t, http.StatusOK, "v2.0.0", &hits)

	if _, _, err := LatestRelease(t.Context(), "owner/repo"); err != nil {
		t.Fatal(err)
	}
	// Age the entry past the TTL.
	releaseCache.Lock()
	e := releaseCache.entries["owner/repo"]
	e.FetchedAt = time.Now().Add(-releaseTTL - time.Minute)
	releaseCache.entries["owner/repo"] = e
	releaseCache.Unlock()

	if _, _, err := LatestRelease(t.Context(), "owner/repo"); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("upstream hit %d times, want 2 (stale entry was not refetched)", hits)
	}
}

// Rate-limited or offline WITH a cached value must fail open: the user still
// sees the last known release, flagged stale, rather than an error banner.
func TestLatestReleaseFailsOpenToStaleCache(t *testing.T) {
	resetCacheForTest(t)
	releaseCache.Lock()
	releaseCache.entries["owner/repo"] = Release{
		Repo: "owner/repo", Tag: "v1.0.0", FetchedAt: time.Now().Add(-releaseTTL - time.Hour),
	}
	releaseCache.Unlock()

	hits := 0
	serveLatest(t, http.StatusForbidden, "", &hits)

	rel, stale, err := LatestRelease(t.Context(), "owner/repo")
	if err != nil {
		t.Fatalf("must fail open with a cached value, got error: %v", err)
	}
	if !stale {
		t.Fatal("served an expired entry without marking it stale")
	}
	if rel.Tag != "v1.0.0" {
		t.Fatalf("tag = %q, want the cached v1.0.0", rel.Tag)
	}
}

// With NO cached value there is nothing honest to report, so the error surfaces.
func TestLatestReleaseErrorsWithoutCache(t *testing.T) {
	resetCacheForTest(t)
	hits := 0
	serveLatest(t, http.StatusForbidden, "", &hits)

	if _, _, err := LatestRelease(t.Context(), "owner/repo"); err == nil {
		t.Fatal("rate-limit with a cold cache must error, not report a fake version")
	}
}

func TestLatestReleaseRejectsEmptyRepo(t *testing.T) {
	resetCacheForTest(t)
	if _, _, err := LatestRelease(t.Context(), ""); err == nil {
		t.Fatal("a tool with no GitHub repo must error rather than fetch nonsense")
	}
}
