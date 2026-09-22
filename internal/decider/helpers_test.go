package decider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Test authorities. The real ones register from internal/agent, which this
// package never imports.
const (
	testGate     = "t-gate"
	testExplicit = "t-explicit"
)

func init() {
	RegisterAuthority(Authority{
		ID: testGate, Group: GroupSafety, Pattern: PatternGate, Label: "Test gate",
		Modes: []Mode{ModeOff, ModeShadow, ModeOn}, DefaultMode: ModeShadow, DefaultThreshold: 0.7,
	})
	RegisterAuthority(Authority{
		ID: testExplicit, Group: GroupFlows, Pattern: PatternPick, Label: "Test explicit",
		Modes: []Mode{ModeOff, ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.6, Explicit: true, FailClosed: true,
	})
}

// noRetryPause makes the one-retry path instant for the duration of a test.
func noRetryPause(t *testing.T) {
	t.Helper()
	prev := retryPause
	retryPause = 0
	t.Cleanup(func() { retryPause = prev })
}

func bearer(key string) func(http.Header) {
	return func(h http.Header) { h.Set("Authorization", "Bearer "+key) }
}

// fakeBox "encrypts" by base64 so tests can see a secret was sealed.
type fakeBox struct{}

func (fakeBox) Encrypt(s string) (string, error) {
	return "enc:" + base64.StdEncoding.EncodeToString([]byte(s)), nil
}

func (fakeBox) Decrypt(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "enc:"))
	return string(raw), err
}

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

// decisionServer speaks the System One wire format on any path. It answers
// every noul with p (default 0.9), every choice with its first option (sorted)
// and every score with level 2, unless status forces an error. It records what
// it received.
type decisionServer struct {
	*httptest.Server
	status   atomic.Int32
	calls    atomic.Int32
	noulP    atomic.Value // float64
	lastAuth atomic.Value
	lastBody atomic.Value
	lastPath atomic.Value
}

func newDecisionServer(t *testing.T) *decisionServer {
	t.Helper()
	ds := &decisionServer{}
	ds.noulP.Store(0.9)
	ds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ds.calls.Add(1)
		ds.lastAuth.Store(r.Header.Get("Authorization"))
		ds.lastPath.Store(r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		ds.lastBody.Store(string(body))
		if s := ds.status.Load(); s != 0 {
			w.WriteHeader(int(s))
			_, _ = fmt.Fprintf(w, `{"error":{"message":"forced %d","code":%d}}`, s, s)
			return
		}
		var req struct {
			Model     string `json:"model"`
			Questions map[string]struct {
				Type     string          `json:"type"`
				Criteria json.RawMessage `json:"criteria"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(body, &req)
		answers := map[string]any{}
		for k, q := range req.Questions {
			switch q.Type {
			case "noul":
				answers[k] = map[string]any{"type": "noul", "noul": ds.noulP.Load().(float64)}
			case "choice":
				var opts map[string]string
				_ = json.Unmarshal(q.Criteria, &opts)
				first := ""
				for _, o := range sortedKeys(opts) {
					first = o
					break
				}
				answers[k] = map[string]any{"type": "choice", "choice": first, "probabilities": map[string]float64{first: 0.8}, "confidence": 0.8}
			case "score":
				answers[k] = map[string]any{"type": "score", "score": 2, "confidence": 0.8}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": req.Model + "-served", "answers": answers, "usage": map[string]any{"input_tokens": 100, "cost": 0.0000042}})
	}))
	t.Cleanup(ds.Close)
	return ds
}

// newTestHub is an in-memory hub, switched on, whose seeded default model
// (DM1: Jev through OpenRouter, borrowed credentials) reaches srv through one
// openrouter provider instance unless other instances are given.
func newTestHub(t *testing.T, srv *decisionServer, instances ...InstanceInfo) (*Hub, *fakeSource) {
	t.Helper()
	noRetryPause(t)
	if len(instances) == 0 {
		instances = []InstanceInfo{{ID: "PRV1", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: true, Available: true}}
	}
	src := &fakeSource{instances: instances}
	h := NewHub(HubOptions{Source: src, Secrets: fakeBox{}})
	cfg := h.Config()
	cfg.Enabled = true
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	return h, src
}

// ownModel adds a decision model with its own endpoint (System One at srv).
func ownModel(t *testing.T, h *Hub, id, base string) ModelInstance {
	t.Helper()
	m, err := h.UpsertModel(ModelInput{ID: id, Backend: SystemOneBackendID, Enabled: true, Model: OpenJevModel, BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func yesNo() Request {
	return Request{State: "state", Questions: map[string]Question{"q": Noul("?", "", "")}}
}

// setAuthority patches one authority's settings.
func setAuthority(t *testing.T, h *Hub, id string, patch func(*AuthorityConfig)) {
	t.Helper()
	cfg := h.Config()
	ac := cfg.Authorities[id]
	patch(&ac)
	cfg.Authorities[id] = ac
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
}
