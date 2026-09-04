package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestHealthCache returns an isolated cache so tests never share verdicts
// with each other or with the package-level one.
func newTestHealthCache(now func() time.Time) *healthCache {
	return &healthCache{
		m:      map[string]healthVerdict{},
		client: &http.Client{Timeout: healthProbeTimeout},
		now:    now,
	}
}

// TestProbeReachableServer covers the happy path: a listening server answers,
// so the endpoint is reachable.
func TestProbeReachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	h := newTestHealthCache(time.Now)
	if !h.probe(context.Background(), srv.URL) {
		t.Error("probe = false against a listening server")
	}
}

// TestProbeCountsAnyHTTPResponse pins that the probe answers "is a server
// listening", not "is this path/credential right". A 401 or 404 still means the
// app is running, and reporting it as down would send the user hunting for a
// closed app that is in fact open.
func TestProbeCountsAnyHTTPResponse(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		h := newTestHealthCache(time.Now)
		if !h.probe(context.Background(), srv.URL) {
			t.Errorf("probe = false for HTTP %d; a responding server is reachable", code)
		}
		srv.Close()
	}
}

// TestProbeUnreachableServer is the case the whole feature exists for: the app
// behind the endpoint is closed, so the connection is refused.
func TestProbeUnreachableServer(t *testing.T) {
	// Bind and immediately close to get a port nothing is listening on.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	h := newTestHealthCache(time.Now)
	if h.probe(context.Background(), url) {
		t.Error("probe = true against a closed server")
	}
}

// TestProbeCachesVerdict pins the caching contract: the UI asks far more often
// than a local server changes state, so repeated asks must not mean repeated
// requests.
func TestProbeCachesVerdict(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	h := newTestHealthCache(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if !h.probe(context.Background(), srv.URL) {
			t.Fatalf("probe %d = false", i)
		}
	}
	if hits != 1 {
		t.Errorf("server saw %d requests, want 1 (verdict must be cached)", hits)
	}

	// Past the TTL the verdict is re-checked, so starting or stopping the server
	// shows up rather than being pinned forever.
	now = now.Add(healthCacheTTL + time.Second)
	if !h.probe(context.Background(), srv.URL) {
		t.Fatal("probe after TTL = false")
	}
	if hits != 2 {
		t.Errorf("server saw %d requests after the TTL expired, want 2", hits)
	}
}

// TestProbeEmptyURL guards the degenerate input: no endpoint is not a reachable
// endpoint, and it must not turn into a request to a bare "/models".
func TestProbeEmptyURL(t *testing.T) {
	h := newTestHealthCache(time.Now)
	if h.probe(context.Background(), "") {
		t.Error("probe(\"\") = true")
	}
}

// TestLocalEndpointReachableOnlyForLocalKinds is the blast-radius guard: a
// hosted provider must report ok=false (not applicable) so no catalog build
// ever probes a remote API.
func TestLocalEndpointReachableOnlyForLocalKinds(t *testing.T) {
	for _, kind := range []string{"anthropic", "openrouter", "deepseek", "openai-compat", "claude-cli"} {
		_, ok := LocalEndpointReachable(context.Background(), kind, ResolvedConfig{BaseURL: "https://api.example.com/v1"})
		if ok {
			t.Errorf("kind %q reported a reachability verdict; hosted liveness must not be probed", kind)
		}
	}
}

// TestLocalEndpointReachableRemoteHost pins that a local KIND aimed at a remote
// host is still somebody else's uptime problem — the decision follows the host,
// exactly like the keyless-auth rule it mirrors.
func TestLocalEndpointReachableRemoteHost(t *testing.T) {
	_, ok := LocalEndpointReachable(context.Background(), "lmstudio", ResolvedConfig{BaseURL: "https://models.example.com/v1"})
	if ok {
		t.Error("a remote-hosted lmstudio instance reported a reachability verdict")
	}
}

// TestLocalEndpointReachableLocalHost covers the real wiring end to end: a
// local kind on a loopback address gets a verdict, and it is accurate.
func TestLocalEndpointReachableLocalHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// httptest binds 127.0.0.1, so this is a genuinely local endpoint.
	localHealth.forget(srv.URL)
	got, ok := LocalEndpointReachable(context.Background(), "lmstudio", ResolvedConfig{BaseURL: srv.URL})
	if !ok {
		t.Fatal("no verdict for a loopback lmstudio instance")
	}
	if !got {
		t.Error("reachable = false while the server was up")
	}

	srv.Close()
	localHealth.forget(srv.URL)
	got, ok = LocalEndpointReachable(context.Background(), "lmstudio", ResolvedConfig{BaseURL: srv.URL})
	if !ok {
		t.Fatal("no verdict after the server closed")
	}
	if got {
		t.Error("reachable = true after the server closed")
	}
}

// TestRegistryReachable covers the Registry seam the API layer actually calls:
// nil for hosted kinds, a real verdict for a local one.
func TestRegistryReachable(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		instanceOf("lmstudio", map[string]string{FieldKeyBaseURL: "http://127.0.0.1:1/v1"}),
		instanceOf("anthropic", map[string]string{FieldKeyAPIKey: "k"}),
	})

	if got := r.Reachable("anthropic"); got != nil {
		t.Errorf("Reachable(anthropic) = %v, want nil (not applicable)", *got)
	}
	got := r.Reachable("lmstudio")
	if got == nil {
		t.Fatal("Reachable(lmstudio) = nil, want a verdict")
	}
	// Port 1 has nothing listening, so the verdict must be false rather than a
	// hopeful true.
	if *got {
		t.Error("Reachable(lmstudio) = true for a port with no listener")
	}
	if r.Reachable("nope") != nil {
		t.Error("Reachable on an unknown instance returned a verdict")
	}
}
