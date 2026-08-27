package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stampVersion sets BuildVersion for the duration of one test. The update check
// reads the package-level build stamp, which is exactly what production does.
func stampVersion(t *testing.T, v string) {
	t.Helper()
	prev := BuildVersion
	BuildVersion = v
	t.Cleanup(func() { BuildVersion = prev })
}

// feedServer serves a latest.json body and counts how many times it was asked,
// so tests can assert on the number of real fetches.
func feedServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/latest.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("update check sent no User-Agent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TIONHARNESS_FEED_URL", srv.URL)
	return srv, &hits
}

const okFeed = `{"version":"0.2.0","released_at":"2026-08-01T00:00:00Z","notes_url":"https://example.test/notes","artifacts":[]}`

func TestUpdateCheckReportsNewerRelease(t *testing.T) {
	stampVersion(t, "0.1.0")
	feedServer(t, http.StatusOK, okFeed)

	got := newUpdateChecker().Status(context.Background(), nil)
	if got.State != "ok" {
		t.Fatalf("state = %q, want ok", got.State)
	}
	if !got.Available {
		t.Fatalf("updateAvailable = false for 0.2.0 over 0.1.0")
	}
	if got.Latest != "0.2.0" || got.Current != "0.1.0" {
		t.Fatalf("versions = %q/%q, want 0.2.0/0.1.0", got.Latest, got.Current)
	}
	if got.NotesURL != "https://example.test/notes" {
		t.Fatalf("notesUrl = %q", got.NotesURL)
	}
}

func TestUpdateCheckReportsNoUpdateWhenCurrent(t *testing.T) {
	stampVersion(t, "0.2.0")
	feedServer(t, http.StatusOK, okFeed)

	got := newUpdateChecker().Status(context.Background(), nil)
	if got.State != "ok" || got.Available {
		t.Fatalf("got %+v, want state=ok with no update", got)
	}
}

// A developer build must never be nagged — and must never even reach out.
func TestUpdateCheckSkipsDevBuild(t *testing.T) {
	stampVersion(t, "dev")
	_, hits := feedServer(t, http.StatusOK, okFeed)

	got := newUpdateChecker().Status(context.Background(), nil)
	if got.State != "skipped" {
		t.Fatalf("state = %q, want skipped", got.State)
	}
	if got.Available {
		t.Fatalf("dev build reported an update: %+v", got)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("dev build hit the feed %d times, want 0", n)
	}
}

// Unreachable / non-200 / malformed all collapse to "unknown" with no update and
// no error surfaced to the caller.
func TestUpdateCheckUnknownOnBadFeed(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"non200", http.StatusInternalServerError, "boom"},
		{"malformed", http.StatusOK, "not json at all"},
		{"noVersion", http.StatusOK, `{"notes_url":"https://example.test"}`},
		{"unparsableVersion", http.StatusOK, `{"version":"banana"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stampVersion(t, "0.1.0")
			feedServer(t, tc.status, tc.body)
			got := newUpdateChecker().Status(context.Background(), nil)
			if got.State != "unknown" || got.Available {
				t.Fatalf("got %+v, want state=unknown with no update", got)
			}
		})
	}
}

func TestUpdateCheckUnknownWhenFeedUnreachable(t *testing.T) {
	stampVersion(t, "0.1.0")
	// A port nothing listens on: the fetch must fail fast and stay silent.
	t.Setenv("TIONHARNESS_FEED_URL", "http://127.0.0.1:1/")
	got := newUpdateChecker().Status(context.Background(), nil)
	if got.State != "unknown" || got.Available {
		t.Fatalf("got %+v, want state=unknown", got)
	}
}

// The cache must hold for the TTL and must not let concurrent callers each open
// their own request.
func TestUpdateCheckCachesAndSinglesFlight(t *testing.T) {
	stampVersion(t, "0.1.0")
	_, hits := feedServer(t, http.StatusOK, okFeed)

	c := newUpdateChecker()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := c.Status(context.Background(), nil); !got.Available {
				t.Errorf("concurrent caller got %+v, want an update", got)
			}
		}()
	}
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("feed fetched %d times under concurrency, want 1", n)
	}
}

func TestUpdateCheckRefetchesAfterTTL(t *testing.T) {
	stampVersion(t, "0.1.0")
	_, hits := feedServer(t, http.StatusOK, okFeed)

	now := time.Unix(1_700_000_000, 0)
	c := newUpdateChecker()
	c.now = func() time.Time { return now }

	c.Status(context.Background(), nil)
	now = now.Add(23 * time.Hour)
	c.Status(context.Background(), nil)
	if n := hits.Load(); n != 1 {
		t.Fatalf("fetched %d times within the TTL, want 1", n)
	}
	now = now.Add(2 * time.Hour) // past 24h
	c.Status(context.Background(), nil)
	if n := hits.Load(); n != 2 {
		t.Fatalf("fetched %d times after the TTL, want 2", n)
	}
}

// TestUpdateCheckAgainstLiveFeed exercises the real feed instead of a stub. It
// is gated on TIONHARNESS_FEED_URL being set (and reachable) because CI has no
// release host: run it against deploy/release-host with
//
//	TIONHARNESS_FEED_URL=http://localhost:8080 go test ./internal/api/ \
//	  -run TestUpdateCheckAgainstLiveFeed -count=1 \
//	  -ldflags "-X github.com/bilal-arikan/tionharness/internal/api.BuildVersion=0.0.0"
//
// The build stamp decides the expectation: a stamped build below the feed must
// see an update, an unstamped "dev" build must see nothing at all.
func TestUpdateCheckAgainstLiveFeed(t *testing.T) {
	feed := os.Getenv("TIONHARNESS_FEED_URL")
	if feed == "" {
		t.Skip("TIONHARNESS_FEED_URL not set; no live release feed to check against")
	}
	got := newUpdateChecker().Status(context.Background(), nil)
	t.Logf("live feed %s -> %+v", feed, got)

	if BuildVersion == devVersion {
		if got.State != "skipped" || got.Available {
			t.Fatalf("dev build got %+v, want a skipped check with no update", got)
		}
		return
	}
	if got.State != "ok" {
		t.Fatalf("state = %q against the live feed, want ok (%+v)", got.State, got)
	}
	if !got.Available {
		t.Fatalf("stamped build %s saw no update from feed version %q", BuildVersion, got.Latest)
	}
}

func TestHandleUpdateCheck(t *testing.T) {
	stampVersion(t, "0.1.0")
	feedServer(t, http.StatusOK, okFeed)

	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/api/version/update", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got updateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if !got.Available || got.Latest != "0.2.0" || got.NotesURL == "" {
		t.Fatalf("handler returned %+v, want an available 0.2.0 with notes", got)
	}
	if got.CheckedAt == "" {
		t.Fatalf("checkedAt is empty")
	}
}
