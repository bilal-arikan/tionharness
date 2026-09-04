package providers

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Local endpoints differ from hosted ones in a way that matters to the UI: the
// server is something the user starts and stops, so a correctly configured
// instance is unusable whenever the app behind it is closed. Available() cannot
// answer that — it is synchronous, called on every catalog build, and must not
// perform I/O — so reachability is tracked separately here and surfaced as its
// own field.
//
// The probe is deliberately cheap and cached: a GET of the endpoint's model
// list with a short deadline, with the verdict reused for healthCacheTTL. The
// UI asks about reachability far more often than a local server changes state.
const (
	// healthProbeTimeout bounds one probe. A loopback server answers in
	// single-digit milliseconds when it is up; when it is down the connection is
	// refused immediately rather than timing out. The budget only has to cover a
	// server that is up but busy loading a model.
	healthProbeTimeout = 800 * time.Millisecond
	// healthCacheTTL is how long a verdict is reused. Short enough that starting
	// the server shows up quickly, long enough that a settings screen polling the
	// provider list does not probe on every render.
	healthCacheTTL = 5 * time.Second
)

// healthVerdict is one cached probe result.
type healthVerdict struct {
	reachable bool
	at        time.Time
}

// healthCache memoises probe results per base URL. Keyed by URL rather than by
// instance id so two instances pointing at the same server share one verdict.
type healthCache struct {
	mu sync.Mutex
	m  map[string]healthVerdict
	// client is shared: probes are tiny and a per-probe client would leak
	// connections when the endpoint is down.
	client *http.Client
	// now is injectable so tests can drive the TTL without sleeping.
	now func() time.Time
}

var localHealth = &healthCache{
	m:      map[string]healthVerdict{},
	client: &http.Client{Timeout: healthProbeTimeout},
	now:    time.Now,
}

// probe reports whether the OpenAI-compatible endpoint at baseURL answers right
// now, reusing a verdict newer than healthCacheTTL. Any HTTP response counts as
// reachable — including 401/404 — because the question is whether a server is
// listening, not whether this particular path or credential is right. Only a
// transport failure (connection refused, DNS, timeout) means unreachable.
func (h *healthCache) probe(ctx context.Context, baseURL string) bool {
	if baseURL == "" {
		return false
	}
	h.mu.Lock()
	if v, ok := h.m[baseURL]; ok && h.now().Sub(v.at) < healthCacheTTL {
		h.mu.Unlock()
		return v.reachable
	}
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	ok := false
	if err == nil {
		resp, rerr := h.client.Do(req)
		if rerr == nil {
			_ = resp.Body.Close()
			ok = true
		}
	}

	h.mu.Lock()
	h.m[baseURL] = healthVerdict{reachable: ok, at: h.now()}
	h.mu.Unlock()
	return ok
}

// forget drops any cached verdict for baseURL, so the next probe hits the
// network. Used by tests and by callers that just changed the endpoint.
func (h *healthCache) forget(baseURL string) {
	h.mu.Lock()
	delete(h.m, baseURL)
	h.mu.Unlock()
}

// LocalEndpointReachable reports whether the local model server for a resolved
// instance config is answering. It returns ok=false for anything that is not a
// local endpoint: a hosted provider's liveness is not this layer's business,
// and probing one on every catalog build would be a real cost.
func LocalEndpointReachable(ctx context.Context, kind string, cfg ResolvedConfig) (reachable, ok bool) {
	if !localProvider(kind) {
		return false, false
	}
	base := cfg.BaseURL
	if base == "" {
		base = lmstudioDefaultBaseURL
	}
	// Reuse the client's own normalisation (trailing slash, MiniMax fallback) so
	// the probe URL matches the one a real request would use.
	c := NewOpenAICompat(kind, cfg.Key, base, cfg.DefaultModel)
	if !c.localEndpoint() {
		// A "local" kind aimed at a remote host is somebody else's uptime problem.
		return false, false
	}
	return localHealth.probe(ctx, c.baseURL), true
}
