package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newLoopbackURLSource builds a urlSource pointed at a test server. The shared
// egress guard refuses loopback, which is exactly right in production but would
// make every test unservable — so the client is swapped for a plain one AFTER
// construction. TestURLSourceRejectsLoopbackThroughEgressGuard covers the guard
// itself, so this substitution cannot hide a missing boundary.
func newLoopbackURLSource(t *testing.T, rawURL string, interval time.Duration) *urlSource {
	t.Helper()
	src, err := NewURLSource(rawURL, interval)
	if err != nil {
		t.Fatalf("NewURLSource: %v", err)
	}
	us := src.(*urlSource)
	us.client = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(us.Close)
	return us
}

// TestURLSourceFiresOnlyWhenBodyChanges is the core contract: the first fetch is
// a silent baseline, an unchanged body is silent, and only a change is an event.
func TestURLSourceFiresOnlyWhenBodyChanges(t *testing.T) {
	body := "status: ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	src := newLoopbackURLSource(t, srv.URL, 0)

	// First poll establishes the baseline and must NOT fire.
	evs, done, _, err := src.Poll(context.Background())
	if err != nil || done {
		t.Fatalf("baseline poll: err=%v done=%v", err, done)
	}
	if len(evs) != 0 {
		t.Fatalf("the baseline fetch fired an event: %+v", evs)
	}

	// Unchanged body: still silent.
	src.next = time.Now()
	if evs, _, _, err = src.Poll(context.Background()); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("an unchanged body fired an event: %+v", evs)
	}

	// Changed body: exactly one event carrying the new content.
	body = "status: FAILED"
	src.next = time.Now()
	if evs, _, _, err = src.Poll(context.Background()); err != nil {
		t.Fatalf("third poll: %v", err)
	}
	if len(evs) != 1 || !strings.Contains(evs[0].Payload, "FAILED") {
		t.Fatalf("a changed body did not fire the new content: %+v", evs)
	}
}

// TestURLSourceRespectsInterval: the manager ticks every second, but the source
// must not hammer a remote host at that rate.
func TestURLSourceRespectsInterval(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Write([]byte("body"))
	}))
	defer srv.Close()

	src := newLoopbackURLSource(t, srv.URL, time.Hour)
	src.Poll(context.Background()) // baseline, consumes the one allowed fetch
	for i := 0; i < 5; i++ {
		src.Poll(context.Background())
	}
	if hits != 1 {
		t.Fatalf("the interval was ignored: %d requests for 6 polls", hits)
	}
}

// TestURLSourceIntervalIsFloored: an agent asking for a 1s poll must be clamped
// to the source minimum rather than granted it.
func TestURLSourceIntervalIsFloored(t *testing.T) {
	src, err := NewURLSource("https://example.com/x", time.Second)
	if err != nil {
		t.Fatalf("NewURLSource: %v", err)
	}
	defer src.Close()
	if got := src.(*urlSource).interval; got != urlSourceMinInterval {
		t.Fatalf("interval %v was not floored to %v", got, urlSourceMinInterval)
	}
}

// TestURLSourceTerminatesAfterRepeatedFailures: an endpoint that is simply gone
// must end the monitor rather than be polled for the rest of the session.
func TestURLSourceTerminatesAfterRepeatedFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening any more

	src := newLoopbackURLSource(t, url, 0)
	var done bool
	var reason string
	for i := 0; i < urlSourceMaxFailures; i++ {
		src.next = time.Now()
		var err error
		_, done, reason, err = src.Poll(context.Background())
		if done {
			break
		}
		if err == nil {
			t.Fatalf("poll %d against a dead server reported no error", i)
		}
	}
	if !done {
		t.Fatalf("the monitor did not terminate after %d failures", urlSourceMaxFailures)
	}
	if !strings.Contains(reason, "unreachable") {
		t.Fatalf("reason %q does not explain the failure", reason)
	}
}

// TestURLSourceRejectsNonHTTPScheme keeps the source honest about what it can
// actually fetch.
func TestURLSourceRejectsNonHTTPScheme(t *testing.T) {
	for _, bad := range []string{"", "ftp://example.com", "wss://example.com", "not a url", "https://"} {
		if _, err := NewURLSource(bad, 0); err == nil {
			t.Fatalf("url %q was accepted", bad)
		}
	}
}

// TestURLSourceRejectsLoopbackThroughEgressGuard proves the guard the production
// client carries: a monitor must not become an SSRF probe into the host.
func TestURLSourceRejectsLoopbackThroughEgressGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("internal"))
	}))
	defer srv.Close()

	src, err := NewURLSource(srv.URL, 0) // keeps the real guarded client
	if err != nil {
		t.Fatalf("NewURLSource: %v", err)
	}
	defer src.Close()
	if _, _, _, err = src.Poll(context.Background()); err == nil {
		t.Fatal("the egress guard allowed a loopback fetch")
	}
}
