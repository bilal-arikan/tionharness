package agent

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

// decisionStub is a fake Decisions endpoint. Each question key gets the answer
// configured for it (noul probability, choice pick, score level); unknown keys
// get a neutral answer. Every request body is recorded.
type decisionStub struct {
	*httptest.Server
	mu     sync.Mutex
	noul   map[string]float64
	choice map[string]string
	status int
	bodies []string
}

func newDecisionStub(t *testing.T) *decisionStub {
	t.Helper()
	s := &decisionStub{noul: map[string]float64{}, choice: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(body))
		status := s.status
		noul := make(map[string]float64, len(s.noul))
		maps.Copy(noul, s.noul)
		choice := make(map[string]string, len(s.choice))
		maps.Copy(choice, s.choice)
		s.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"stubbed failure"}}`)
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
				p, ok := noul[k]
				if !ok {
					p = 0.5
				}
				answers[k] = map[string]any{"type": "noul", "noul": p}
			case "choice":
				c := choice[k]
				answers[k] = map[string]any{"type": "choice", "choice": c, "probabilities": map[string]float64{c: 0.9}, "confidence": 0.8}
			case "score":
				answers[k] = map[string]any{"type": "score", "score": 2, "confidence": 0.9}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "typesafe/jev-1.13-20260917",
			"answers": answers,
			"usage":   map[string]any{"input_tokens": 500, "output_tokens": 40, "cost": 0.000021},
		})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *decisionStub) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *decisionStub) set(fn func(s *decisionStub)) {
	s.mu.Lock()
	fn(s)
	s.mu.Unlock()
}

// wireDecider enables the decider on tun against stub, with the given
// authorities at mode. The hub's seeded default model (Jev through OpenRouter)
// is re-pointed at the stub with a test key.
func wireDecider(t *testing.T, tun *Tunables, stub *decisionStub, modes map[string]decider.Mode) *decider.Hub {
	t.Helper()
	hub := newStubDeciderHub(t, stub.URL)
	cfg := hub.Config()
	cfg.Enabled = true
	for id, m := range modes {
		ac := cfg.Authorities[id]
		ac.Mode = m
		cfg.Authorities[id] = ac
	}
	if _, err := hub.Update(cfg); err != nil {
		t.Fatal(err)
	}
	tun.SetDecider(hub)
	return hub
}

// plainSecretBox stores decision-model keys unencrypted; tests only need the
// key to round-trip into the Authorization header.
type plainSecretBox struct{}

func (plainSecretBox) Encrypt(s string) (string, error) { return s, nil }
func (plainSecretBox) Decrypt(s string) (string, error) { return s, nil }

// newStubDeciderHub builds an in-memory hub whose seeded DM1 model answers
// from the decision stub at url (own credentials, test key).
func newStubDeciderHub(t *testing.T, url string) *decider.Hub {
	t.Helper()
	hub := decider.NewHub(decider.HubOptions{Secrets: plainSecretBox{}})
	seed, ok := hub.Model("DM1")
	if !ok {
		t.Fatal("hub did not seed DM1")
	}
	if _, err := hub.UpsertModel(decider.ModelInput{ID: seed.ID, Backend: seed.Backend, Label: seed.Label, Enabled: true, Model: seed.Model,
		BaseURL: url + "/api/v1", TimeoutMs: seed.TimeoutMs, Secrets: map[string]string{decider.SecretAPIKey: "test-key"}}); err != nil {
		t.Fatal(err)
	}
	return hub
}

// waitBackground waits for background (shadow) decisions to finish.
func waitBackground(rt *Runtime) { rt.spawnWG.Wait() }

func TestDecideBillsCallerUnderBackendProvider(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	wireDecider(t, tun, stub, map[string]decider.Mode{authToolRisk: decider.ModeOn})
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.decide(ctx, authToolRisk, caller, toolRiskRequest("Bash", "git push --force origin main")); err != nil {
		t.Fatal(err)
	}
	u, err := rt.db.GetUsageToday(ctx, caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st := u.ByModel[db.ModelKey("openrouter", decider.JevModel)]; st.Calls != 1 || st.InputTokens != 500 {
		t.Errorf("usage by model = %+v; the call must be billed as openrouter/%s (the requested id, not the served snapshot)", u.ByModel, decider.JevModel)
	}
	if st := u.ByKind[db.UsageKindDecide]; st.Calls != 1 {
		t.Errorf("usage by kind = %+v, want one %q call", u.ByKind, db.UsageKindDecide)
	}
}

func TestDecideChallengerIsBilledAndSpeaksTheAuthorityVocabulary(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{authToolRisk: decider.ModeOn})
	rival, err := hub.UpsertModel(decider.ModelInput{Backend: decider.SystemOneBackendID, Label: "local", Enabled: true, Model: decider.OpenJevModel, BaseURL: stub.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := hub.Config()
	ac := cfg.Authorities[authToolRisk]
	ac.Challenger = rival.ID
	cfg.Authorities[authToolRisk] = ac
	if _, err := hub.Update(cfg); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.95 })
	if flagged, _ := rt.ToolRiskFlagged(ctx, caller, "Bash", json.RawMessage(`{"command":"git push --force origin main"}`), true); !flagged {
		t.Fatal("the primary decision must flag the push for approval")
	}
	waitBackground(rt)
	var challenger *decider.Record
	for _, r := range hub.Recent(5) {
		if r.Role == decider.RoleChallenger {
			challenger = &r
		}
	}
	if challenger == nil || challenger.Instance != rival.ID || challenger.Outcome != "ask" || challenger.Baseline != "ask" || !challenger.Agrees() {
		t.Fatalf("challenger record = %+v", challenger)
	}
	u, err := rt.db.GetUsageToday(ctx, caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st := u.ByKind[db.UsageKindDecide]; st.Calls != 2 {
		t.Errorf("decide calls = %d, want the primary and the challenger", st.Calls)
	}
	if st := u.ByModel[db.ModelKey("local", decider.OpenJevModel)]; st.Calls != 1 {
		t.Errorf("usage by model = %+v; the local challenger bills under the free \"local\" provider", u.ByModel)
	}
}
