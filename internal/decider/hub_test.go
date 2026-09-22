package decider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSource is an EndpointSource over a fixed instance list.
type fakeSource struct {
	mu        sync.Mutex
	instances []InstanceInfo
	gen       uint64
	resolved  atomic.Int32
}

func (f *fakeSource) Endpoint(id string) (Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved.Add(1)
	for _, inst := range f.instances {
		if inst.ID == id {
			return Endpoint{InstanceID: id, Kind: inst.Kind, BaseURL: inst.BaseURL, Authorize: bearer("key-" + id)}, nil
		}
	}
	return Endpoint{}, fmt.Errorf("unknown provider instance: %q", id)
}

func (f *fakeSource) Instances() []InstanceInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]InstanceInfo(nil), f.instances...)
}

func (f *fakeSource) Generation() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gen
}

func (f *fakeSource) bump() {
	f.mu.Lock()
	f.gen++
	f.mu.Unlock()
}

// decisionServer answers every question "yes" with p=0.9 unless status forces
// an error, and records what it received.
type decisionServer struct {
	*httptest.Server
	status   atomic.Int32
	calls    atomic.Int32
	lastAuth atomic.Value
	lastBody atomic.Value
}

func newDecisionServer(t *testing.T) *decisionServer {
	t.Helper()
	ds := &decisionServer{}
	ds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ds.calls.Add(1)
		ds.lastAuth.Store(r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		ds.lastBody.Store(string(body))
		if s := ds.status.Load(); s != 0 {
			w.WriteHeader(int(s))
			_, _ = fmt.Fprintf(w, `{"error":{"message":"forced %d","code":%d}}`, s, s)
			return
		}
		var req struct {
			Questions map[string]struct {
				Type string `json:"type"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(body, &req)
		answers := map[string]any{}
		for k, q := range req.Questions {
			switch q.Type {
			case "noul":
				answers[k] = map[string]any{"type": "noul", "noul": 0.9}
			case "score":
				answers[k] = map[string]any{"type": "score", "score": 2, "confidence": 0.8}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]any{"input_tokens": 100, "cost": 0.0000042}})
	}))
	t.Cleanup(ds.Close)
	return ds
}

func newTestHub(t *testing.T, srv *decisionServer, instances ...InstanceInfo) (*Hub, *fakeSource) {
	t.Helper()
	noRetryPause(t)
	if len(instances) == 0 {
		instances = []InstanceInfo{{ID: "PRV1", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: true, Available: true}}
	}
	src := &fakeSource{instances: instances}
	h := NewHub(HubOptions{Source: src})
	cfg := DefaultConfig()
	cfg.Enabled = true
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	return h, src
}

func yesNo() Request {
	return Request{State: "state", Questions: map[string]Question{"q": Noul("?", "", "")}}
}

func TestHubDecideRespectsSwitches(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)

	resp, err := h.Decide(context.Background(), SiteStallJudge, yesNo())
	if err != nil || !resp.Answers["q"].Yes(0.5) {
		t.Fatalf("decide = %+v, %v", resp, err)
	}
	if srv.lastAuth.Load() != "Bearer key-PRV1" {
		t.Errorf("auth = %v", srv.lastAuth.Load())
	}

	cfg := h.Config()
	cfg.Sites[SiteStallJudge] = SiteConfig{Mode: ModeOff}
	_, _ = h.Update(cfg)
	if _, err := h.Decide(context.Background(), SiteStallJudge, yesNo()); !errors.Is(err, ErrSiteOff) {
		t.Errorf("site off: err = %v", err)
	}
	cfg.Enabled = false
	_, _ = h.Update(cfg)
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); !errors.Is(err, ErrDisabled) {
		t.Errorf("master off: err = %v", err)
	}
	// The settings screen's test works while switched off.
	if _, err := h.Test(context.Background()); err != nil {
		t.Errorf("test while disabled: %v", err)
	}
	var nilHub *Hub
	if _, err := nilHub.Decide(context.Background(), SiteToolRisk, yesNo()); !errors.Is(err, ErrDisabled) {
		t.Errorf("nil hub: err = %v", err)
	}
}

func TestHubPicksCompatibleInstanceAndCachesClient(t *testing.T) {
	srv := newDecisionServer(t)
	h, src := newTestHub(t, srv,
		InstanceInfo{ID: "anth", Kind: "anthropic", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV9", Kind: "openai-compat", BaseURL: "https://openrouter.ai/api/v1", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV2", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV1", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: false, Available: true},
	)
	if c := h.Candidates(); len(c) != 2 || c[0].ID != "PRV2" || c[1].ID != "PRV9" {
		t.Fatalf("candidates = %+v, want the enabled openrouter instance first, then the openai-compat one", c)
	}
	for range 3 {
		if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); err != nil {
			t.Fatal(err)
		}
	}
	if n := src.resolved.Load(); n != 1 {
		t.Errorf("endpoint resolved %d times, want 1 (client cached)", n)
	}
	src.bump() // providers re-saved: the client must be rebuilt
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); err != nil {
		t.Fatal(err)
	}
	if n := src.resolved.Load(); n != 2 {
		t.Errorf("endpoint resolved %d times after a generation bump, want 2", n)
	}
}

func TestHubNoEndpoint(t *testing.T) {
	h := NewHub(HubOptions{Source: &fakeSource{}})
	cfg := DefaultConfig()
	cfg.Enabled = true
	_, _ = h.Update(cfg)
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); !errors.Is(err, ErrNoEndpoint) {
		t.Errorf("err = %v, want ErrNoEndpoint", err)
	}
	if st := h.Status(); st.Ready || st.Problem == "" {
		t.Errorf("status = %+v, want not ready with a problem", st)
	}
}

func TestHubQuarantinesRejectedCredentials(t *testing.T) {
	srv := newDecisionServer(t)
	h, src := newTestHub(t, srv)
	srv.status.Store(http.StatusUnauthorized)
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); !IsAuthError(err) {
		t.Fatalf("err = %v, want an auth error", err)
	}
	before := srv.calls.Load()
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); !errors.Is(err, ErrBackoff) {
		t.Fatalf("second call err = %v, want ErrBackoff (quarantined)", err)
	}
	if srv.calls.Load() != before {
		t.Error("a quarantined endpoint was called again")
	}
	if st := h.Status(); st.Ready || st.BackoffUntil == 0 {
		t.Errorf("status = %+v, want a quarantine", st)
	}
	// Re-saving providers (new generation) lifts the quarantine at once.
	srv.status.Store(0)
	src.bump()
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); err != nil {
		t.Errorf("after re-saving providers: %v", err)
	}
}

func TestHubCircuitBreaker(t *testing.T) {
	srv := newDecisionServer(t)
	now := time.Now()
	var mu sync.Mutex
	h, _ := newTestHub(t, srv)
	h.opts.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	srv.status.Store(http.StatusServiceUnavailable)
	for i := range breakerThreshold {
		if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); err == nil || errors.Is(err, ErrBackoff) {
			t.Fatalf("failure %d: err = %v", i, err)
		}
	}
	calls := srv.calls.Load()
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); !errors.Is(err, ErrBackoff) {
		t.Fatalf("err = %v, want ErrBackoff once the circuit is open", err)
	}
	if srv.calls.Load() != calls {
		t.Error("the endpoint was called while the circuit was open")
	}
	srv.status.Store(0)
	mu.Lock()
	now = now.Add(breakerCooldown + time.Second)
	mu.Unlock()
	if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); err != nil {
		t.Errorf("after cooldown: %v", err)
	}
	// A 400 is about the request, not the endpoint: it never opens the circuit.
	srv.status.Store(http.StatusBadRequest)
	for range breakerThreshold + 1 {
		if _, err := h.Decide(context.Background(), SiteToolRisk, yesNo()); errors.Is(err, ErrBackoff) {
			t.Fatal("bad requests opened the circuit")
		}
	}
}

func TestHubRedactsAndBoundsState(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	long := strings.Repeat("x", 200_000) + " " + secret
	if _, err := h.Decide(context.Background(), SiteToolRisk, Request{State: long, Questions: yesNo().Questions}); err != nil {
		t.Fatal(err)
	}
	body := srv.lastBody.Load().(string)
	if strings.Contains(body, "abcdefghijklmnop") {
		t.Error("secret reached the decision service")
	}
	if len(body) > 90_000 {
		t.Errorf("request body is %d bytes; state was not bounded", len(body))
	}
	// Structured state that does not fit is refused rather than cut.
	huge := map[string]any{"blob": strings.Repeat("y", 200_000)}
	if _, err := h.Decide(context.Background(), SiteToolRisk, Request{State: huge, Questions: yesNo().Questions}); err == nil {
		t.Error("oversized structured state was sent")
	}
}

func TestHubUpdatePersists(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}})
	cfg := h.Config()
	cfg.Enabled = true
	cfg.Model = JevLatestModel
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	again := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}})
	if c := again.Config(); !c.Enabled || c.Model != JevLatestModel {
		t.Errorf("reloaded config = %+v", c)
	}
	if _, err := h.Update(Config{Backend: "nope"}); err == nil {
		t.Error("invalid config accepted")
	}
	h.Log(Record{Site: SiteToolRisk, Mode: ModeShadow, Outcome: "run", Baseline: "run"})
	if st := h.Stats(time.Hour); len(st) != 1 || st[0].Agreed != 1 {
		t.Errorf("stats = %+v", st)
	}
}
