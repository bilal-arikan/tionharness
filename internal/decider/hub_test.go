package decider

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHubDecideRespectsSwitches(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)

	resp, err := h.Decide(context.Background(), testGate, yesNo())
	if err != nil || !resp.Answers["q"].Yes(0.5) {
		t.Fatalf("decide = %+v, %v", resp, err)
	}
	if resp.Instance != "DM1" || srv.lastAuth.Load() != "Bearer key-PRV1" {
		t.Errorf("instance = %q auth = %v; want the seeded model on the borrowed key", resp.Instance, srv.lastAuth.Load())
	}
	if srv.lastPath.Load() != "/api/alpha/decisions" {
		t.Errorf("path = %v", srv.lastPath.Load())
	}

	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOff })
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrSiteOff) {
		t.Errorf("authority off: err = %v", err)
	}
	cfg := h.Config()
	cfg.Enabled = false
	_, _ = h.Update(cfg)
	if _, err := h.Decide(context.Background(), testExplicit, Request{State: "s", Questions: map[string]Question{
		"a": Choice("?", map[string]string{"x": "X", "y": "Y"}),
	}}); !errors.Is(err, ErrDisabled) {
		t.Errorf("master off: err = %v", err)
	}
	// The settings screen's test works while switched off.
	if _, err := h.Test(context.Background(), ""); err != nil {
		t.Errorf("test while disabled: %v", err)
	}
	var nilHub *Hub
	if _, err := nilHub.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrDisabled) {
		t.Errorf("nil hub: err = %v", err)
	}
	if nilHub.Mode(testGate) != ModeOff {
		t.Error("nil hub reports a mode")
	}
}

func TestHubBorrowedCredentialsPickAndCache(t *testing.T) {
	srv := newDecisionServer(t)
	h, src := newTestHub(t, srv,
		InstanceInfo{ID: "anth", Kind: "anthropic", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV9", Kind: "openai-compat", BaseURL: "https://openrouter.ai/api/v1", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV2", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: true, Available: true},
		InstanceInfo{ID: "PRV1", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Enabled: false, Available: true},
	)
	if c := h.ProviderCandidates(OpenRouterBackendID); len(c) != 2 || c[0].ID != "PRV2" || c[1].ID != "PRV9" {
		t.Fatalf("candidates = %+v, want the enabled openrouter instance first, then the openai-compat one", c)
	}
	if c := h.ProviderCandidates(LogprobsBackendID); len(c) != 2 {
		t.Errorf("logprobs candidates = %+v, want both chat-completions instances", c)
	}
	for range 3 {
		if _, err := h.Decide(context.Background(), testGate, yesNo()); err != nil {
			t.Fatal(err)
		}
	}
	if n := src.resolved.Load(); n != 1 {
		t.Errorf("endpoint resolved %d times, want 1 (client cached)", n)
	}
	src.bump() // providers re-saved: the client must be rebuilt
	if _, err := h.Decide(context.Background(), testGate, yesNo()); err != nil {
		t.Fatal(err)
	}
	if n := src.resolved.Load(); n != 2 {
		t.Errorf("endpoint resolved %d times after a generation bump, want 2", n)
	}
	if st := h.ModelStatus("DM1"); !st.Ready || st.Provider != "PRV2" {
		t.Errorf("status = %+v", st)
	}
}

func TestHubOwnCredentialsAndModelBinding(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)
	m, err := h.UpsertModel(ModelInput{
		Backend: SystemOneBackendID, Label: "OpenJev", Enabled: true, Model: OpenJevModel,
		BaseURL: srv.URL + "/v1", Secrets: map[string]string{SecretAPIKey: "local-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "DM2" || m.SecretsEnc[SecretAPIKey] == "local-secret" || m.SecretsEnc[SecretAPIKey] == "" {
		t.Fatalf("model = %+v; want DM2 with a sealed key", m)
	}
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Model = m.ID })
	resp, err := h.Decide(context.Background(), testGate, yesNo())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Instance != "DM2" || resp.Model != OpenJevModel || resp.Backend != SystemOneBackendID {
		t.Errorf("resp = %+v", resp)
	}
	if srv.lastAuth.Load() != "Bearer local-secret" || srv.lastPath.Load() != "/v1/systemone" {
		t.Errorf("auth = %v path = %v", srv.lastAuth.Load(), srv.lastPath.Load())
	}
	if resp.BillingProvider != billingLocal {
		t.Errorf("billing = %q, want local for a loopback server", resp.BillingProvider)
	}
	if got := h.EffectiveModel(testGate); got != "DM2" {
		t.Errorf("effective model = %q", got)
	}
	// A disabled model does not answer for an authority, but can be tested.
	in := ModelInput{ID: m.ID, Backend: m.Backend, Label: m.Label, Enabled: false, Model: m.Model, BaseURL: m.BaseURL}
	if _, err := h.UpsertModel(in); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrModelDisabled) {
		t.Errorf("disabled model: err = %v", err)
	}
	if _, err := h.Test(context.Background(), m.ID); err != nil {
		t.Errorf("testing a disabled model: %v", err)
	}
	if srv.lastAuth.Load() != "Bearer local-secret" {
		t.Error("updating the model without secrets dropped its key")
	}
}

func TestHubNoEndpointAndNoModel(t *testing.T) {
	h := NewHub(HubOptions{Source: &fakeSource{}, Secrets: fakeBox{}})
	cfg := h.Config()
	cfg.Enabled = true
	_, _ = h.Update(cfg)
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrNoEndpoint) {
		t.Errorf("err = %v, want ErrNoEndpoint", err)
	}
	if st := h.Status(); st.Ready || st.Problem == "" || st.Model != "DM1" {
		t.Errorf("status = %+v, want DM1 not ready with a problem", st)
	}
	if _, err := h.DeleteModel("DM1"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel once no model exists", err)
	}
	if st := h.Status(); st.Ready || st.Model != "" {
		t.Errorf("status = %+v", st)
	}
}

func TestHubQuarantineIsPerModel(t *testing.T) {
	srv := newDecisionServer(t)
	good := newDecisionServer(t)
	h, src := newTestHub(t, srv)
	ownModel(t, h, "local", good.URL+"/v1")
	setAuthority(t, h, testExplicit, func(ac *AuthorityConfig) { ac.Model = "local" })
	srv.status.Store(http.StatusUnauthorized)
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !IsAuthError(err) {
		t.Fatalf("err = %v, want an auth error", err)
	}
	before := srv.calls.Load()
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrBackoff) {
		t.Fatalf("second call err = %v, want ErrBackoff (quarantined)", err)
	}
	if srv.calls.Load() != before {
		t.Error("a quarantined model was called again")
	}
	if st := h.ModelStatus("DM1"); st.Ready || st.BackoffUntil == 0 {
		t.Errorf("status = %+v, want a quarantine", st)
	}
	// Another model is not affected.
	if _, err := h.Decide(context.Background(), testExplicit, yesNo()); err != nil {
		t.Errorf("healthy model refused while another is quarantined: %v", err)
	}
	// Re-saving the providers (new generation) lifts the quarantine of a model
	// borrowing their key.
	srv.status.Store(0)
	src.bump()
	if _, err := h.Decide(context.Background(), testGate, yesNo()); err != nil {
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
		if _, err := h.Decide(context.Background(), testGate, yesNo()); err == nil || errors.Is(err, ErrBackoff) {
			t.Fatalf("failure %d: err = %v", i, err)
		}
	}
	calls := srv.calls.Load()
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrBackoff) {
		t.Fatalf("err = %v, want ErrBackoff once the circuit is open", err)
	}
	if srv.calls.Load() != calls {
		t.Error("the model was called while the circuit was open")
	}
	srv.status.Store(0)
	mu.Lock()
	now = now.Add(breakerCooldown + time.Second)
	mu.Unlock()
	if _, err := h.Decide(context.Background(), testGate, yesNo()); err != nil {
		t.Errorf("after cooldown: %v", err)
	}
	// A 400 is about the request, not the model: it never opens the circuit.
	srv.status.Store(http.StatusBadRequest)
	for range breakerThreshold + 1 {
		if _, err := h.Decide(context.Background(), testGate, yesNo()); errors.Is(err, ErrBackoff) {
			t.Fatal("bad requests opened the circuit")
		}
	}
}

func TestHubFallback(t *testing.T) {
	primary := newDecisionServer(t)
	backup := newDecisionServer(t)
	h, _ := newTestHub(t, primary)
	ownModel(t, h, "backup", backup.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Fallback = "backup" })

	primary.status.Store(http.StatusServiceUnavailable)
	resp, err := h.Decide(context.Background(), testGate, yesNo())
	if err != nil {
		t.Fatalf("fallback did not answer: %v", err)
	}
	if !resp.Fallback || resp.Instance != "backup" {
		t.Errorf("resp = %+v, want the fallback's answer marked as such", resp)
	}
	if rec := NewRecord(testGate, ModeOn, resp, nil); !rec.Fallback || rec.Instance != "backup" {
		t.Errorf("record = %+v", rec)
	}

	// A malformed request is not retried elsewhere.
	calls := backup.calls.Load()
	if _, err := h.Decide(context.Background(), testGate, Request{State: "s"}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("err = %v, want ErrInvalidRequest", err)
	}
	if backup.calls.Load() != calls {
		t.Error("an invalid request went to the fallback")
	}

	// Both failing: the error names both.
	backup.status.Store(http.StatusServiceUnavailable)
	_, err = h.Decide(context.Background(), testGate, yesNo())
	if err == nil || !strings.Contains(err.Error(), "openrouter-decisions") || !strings.Contains(err.Error(), "systemone") {
		t.Errorf("err = %v, want both failures", err)
	}
	var me *ModelError
	if !errors.As(err, &me) || me.Instance != "DM1" {
		t.Errorf("model error = %+v", me)
	}
}

func TestHubChallenger(t *testing.T) {
	primary := newDecisionServer(t)
	rival := newDecisionServer(t)
	rival.noulP.Store(0.2) // disagrees at any threshold above 0.2
	h, _ := newTestHub(t, primary)
	ownModel(t, h, "rival", rival.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn; ac.Challenger = "rival" })

	var billedMu sync.Mutex
	var billed []string
	var wg sync.WaitGroup
	resp, err := h.Decide(context.Background(), testGate, yesNo(),
		WithBilling(func(_ context.Context, r *Response) {
			billedMu.Lock()
			billed = append(billed, r.Instance)
			billedMu.Unlock()
		}),
		WithBackground(func(f func()) { ; wg.Go(func() { ; f() }) }),
		WithRef("SES1"),
	)
	if err != nil || resp.Instance != "DM1" {
		t.Fatalf("decide = %+v, %v", resp, err)
	}
	wg.Wait()
	recs := h.Recent(5)
	if len(recs) != 1 {
		t.Fatalf("ledger = %+v, want one challenger record", recs)
	}
	r := recs[0]
	if r.Role != RoleChallenger || r.Instance != "rival" || r.Baseline != "yes" || r.Outcome != "no" || r.Agrees() || r.Ref != "SES1" {
		t.Errorf("challenger record = %+v", r)
	}
	billedMu.Lock()
	if len(billed) != 2 || billed[0] != "DM1" || billed[1] != "rival" {
		t.Errorf("billed = %v, want the primary and the challenger", billed)
	}
	billedMu.Unlock()
	st := h.Stats(time.Hour)
	if len(st) != 1 || st[0].ChallengerCompared != 1 || st[0].ChallengerAgreed != 0 || st[0].Challenger != "rival" || st[0].Calls != 0 {
		t.Errorf("stats = %+v", st)
	}
	if ms := h.ModelStats(time.Hour); len(ms) != 1 || ms[0].Instance != "rival" {
		t.Errorf("model stats = %+v", ms)
	}

	// An authority-specific vocabulary is used on both sides.
	wg = sync.WaitGroup{}
	_, err = h.Decide(context.Background(), testGate, yesNo(),
		WithOutcome(func(r *Response) (string, float64) {
			if r.Answers["q"].Yes(0.5) {
				return "stalled", r.Answers["q"].Probability
			}
			return "ok", r.Answers["q"].Probability
		}),
		WithBackground(func(f func()) { ; wg.Go(func() { ; f() }) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if r := h.Recent(1)[0]; r.Baseline != "stalled" || r.Outcome != "ok" {
		t.Errorf("vocabulary record = %+v", r)
	}
}

func TestHubRedactsAndBoundsState(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	long := strings.Repeat("x", 200_000) + " " + secret
	if _, err := h.Decide(context.Background(), testGate, Request{State: long, Questions: yesNo().Questions}); err != nil {
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
	if _, err := h.Decide(context.Background(), testGate, Request{State: huge, Questions: yesNo().Questions}); err == nil {
		t.Error("oversized structured state was sent")
	}
}

func TestHubUpdatePersistsAndChecksModels(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}, Secrets: fakeBox{}})
	if ms := h.Models(); len(ms) != 1 || ms[0].ID != "DM1" || ms[0].Credentials != CredentialsProvider {
		t.Fatalf("seeded models = %+v", ms)
	}
	m := ownModel(t, h, "", "http://127.0.0.1:9/v1")
	cfg := h.Config()
	cfg.Enabled = true
	cfg.DefaultModel = m.ID
	ac := cfg.Authorities[testGate]
	ac.Challenger = "DM1"
	cfg.Authorities[testGate] = ac
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	again := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}, Secrets: fakeBox{}})
	if c := again.Config(); !c.Enabled || c.DefaultModel != m.ID || c.Authorities[testGate].Challenger != "DM1" {
		t.Errorf("reloaded config = %+v", c)
	}
	if ms := again.Models(); len(ms) != 2 {
		t.Errorf("reloaded models = %+v; a deleted or existing list must never be re-seeded", ms)
	}

	bad := h.Config()
	bad.DefaultModel = "nope"
	if _, err := h.Update(bad); err == nil {
		t.Error("a config naming an unknown model was accepted")
	}
	bad = h.Config()
	bad.Authorities["mystery"] = AuthorityConfig{Mode: ModeOn}
	if _, err := h.Update(bad); err == nil {
		t.Error("an unknown authority was accepted")
	}

	users, err := h.DeleteModel("DM1")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0] != testGate {
		t.Errorf("users = %v", users)
	}
	if c := h.Config(); c.Authorities[testGate].Challenger != "" {
		t.Errorf("deleted model still referenced: %+v", c.Authorities[testGate])
	}
	users, _ = h.DeleteModel(m.ID)
	if len(users) != 1 || users[0] != "default" || h.Config().DefaultModel != "" {
		t.Errorf("users = %v default = %q", users, h.Config().DefaultModel)
	}
	if ms := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}}).Models(); len(ms) != 0 {
		t.Errorf("an emptied model list was re-seeded: %+v", ms)
	}

	h.Log(Record{Authority: testGate, Mode: ModeShadow, Outcome: "run", Baseline: "run"})
	if st := h.Stats(time.Hour); len(st) != 1 || st[0].Agreed != 1 {
		t.Errorf("stats = %+v", st)
	}
}

func TestHubConvertsVersionOneConfig(t *testing.T) {
	dir := t.TempDir()
	v1 := `{"enabled":true,"backend":"openrouter","providerInstanceId":"PRV3","model":"~typesafe/jev-latest","timeoutMs":4500,
		"sites":{"t-gate":{"mode":"on","threshold":0.75}}}`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}})
	ms := h.Models()
	if len(ms) != 1 {
		t.Fatalf("models = %+v", ms)
	}
	m := ms[0]
	if m.ID != "DM1" || m.Backend != OpenRouterBackendID || m.Credentials != CredentialsProvider ||
		m.ProviderInstanceID != "PRV3" || m.Model != JevLatestModel || m.TimeoutMs != 4500 || !m.Enabled {
		t.Errorf("converted model = %+v", m)
	}
	c := h.Config()
	if !c.Enabled || c.Authorities[testGate].Mode != ModeOn || c.Authorities[testGate].Threshold != 0.75 {
		t.Errorf("converted config = %+v", c)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, configFileName))
	if !strings.Contains(string(raw), `"version": 2`) || strings.Contains(string(raw), "sites") {
		t.Errorf("config not rewritten as version 2:\n%s", raw)
	}
	// Loading again converts nothing twice.
	if again := NewHub(HubOptions{DataDir: dir, Source: &fakeSource{}}); len(again.Models()) != 1 {
		t.Errorf("second load changed the models: %+v", again.Models())
	}
}

func TestFillCostPricesUnpricedCalls(t *testing.T) {
	m := systemOneBackend{}.Manifest()
	resp := &Response{Model: JevNativeModel, BillingProvider: billingTypeSafe, Usage: Usage{InputTokens: 1_000_000}}
	fillCost(m, resp)
	if resp.Usage.CostUSD != jevInputPerMTok {
		t.Errorf("cost = %v", resp.Usage.CostUSD)
	}
	local := &Response{Model: JevNativeModel, BillingProvider: billingLocal, Usage: Usage{InputTokens: 1_000_000}}
	fillCost(m, local)
	if local.Usage.CostUSD != 0 {
		t.Errorf("a local call was priced: %v", local.Usage.CostUSD)
	}
}
