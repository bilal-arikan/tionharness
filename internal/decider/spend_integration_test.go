package decider

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// spendWireServer exercises the real HTTP decoder and Hub accounting together.
// A nil price omits usage.cost; a pointer to zero explicitly reports free usage.
func spendWireServer(t *testing.T, cost *float64, malformed bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req soRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		answers := map[string]any{}
		for key, q := range req.Questions {
			switch q.Type {
			case "noul":
				p := 0.9
				if malformed && (key == "bad" || key == "q") {
					p = 1.2
				}
				answers[key] = map[string]any{"type": "noul", "noul": p}
			case "score":
				answers[key] = map[string]any{"type": "score", "score": 2, "confidence": 0.9}
			}
		}
		usage := map[string]any{"input_tokens": 1000, "output_tokens": 7}
		if cost != nil {
			usage["cost"] = *cost
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "served-snapshot", "answers": answers, "usage": usage})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func spendHTTPHub(t *testing.T, srv *httptest.Server) *Hub {
	t.Helper()
	h, _ := newTestHub(t, &decisionServer{Server: srv})
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn })
	return h
}

func assertSpendCost(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("cost = %.12g, want %.12g", got, want)
	}
}

func TestHubSpendPrimaryPersistsWithoutCallerBilling(t *testing.T) {
	price := 0.0000123
	srv, calls := spendWireServer(t, &price, false)
	h := spendHTTPHub(t, srv)
	dir := t.TempDir()
	h.spends = openSpendStore(dir, h.logger, h.now())
	resp, err := h.Decide(context.Background(), testGate, yesNo())
	if err != nil || resp == nil {
		t.Fatalf("decide = %+v, %v", resp, err)
	}
	report := h.Spend(7)
	if calls.Load() != 1 || report.Totals.Calls != 1 || report.Totals.PrimaryCalls != 1 || report.Totals.InputTokens != 1000 || report.Totals.OutputTokens != 7 || report.Totals.UnknownCostCalls != 0 {
		t.Fatalf("calls = %d, spend = %+v", calls.Load(), report)
	}
	assertSpendCost(t, report.Totals.CostUSD, price)
	assertSpendCost(t, report.Totals.ReportedCostUSD, price)
	if len(report.Models) != 1 || report.Models[0].Provider != billingOpenRouter || report.Models[0].Model != JevModel {
		t.Fatalf("billing identity = %+v", report.Models)
	}
	reopened := openSpendStore(dir, h.logger, time.Now()).report(time.Now(), 7)
	if reopened.Totals != report.Totals || reopened.StorageError {
		t.Fatalf("persisted spend = %+v, want %+v", reopened, report)
	}
}

func TestHubSpendRejectsChargedMalformedAnswerButKeepsItsUsage(t *testing.T) {
	price := 0.00004
	srv, calls := spendWireServer(t, &price, true)
	h := spendHTTPHub(t, srv)
	var billed *Response
	req := Request{State: "test", Questions: map[string]Question{"good": Noul("good?", "", ""), "bad": Noul("bad?", "", "")}}
	resp, err := h.Decide(context.Background(), testGate, req, WithBilling(func(_ context.Context, r *Response) { billed = r }))
	if resp != nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("malformed answer escaped Hub: response=%+v error=%v", resp, err)
	}
	if billed == nil || billed.Usage.InputTokens != 1000 || billed.Usage.CostSource != "reported" || len(billed.Answers) != 0 {
		t.Fatalf("billing must receive usage only: %+v", billed)
	}
	report := h.Spend(7)
	if calls.Load() != 1 || report.Totals.Calls != 1 || report.Totals.Failures != 1 || report.Totals.PrimaryCalls != 1 || report.Totals.InputTokens != 1000 {
		t.Fatalf("malformed charged call spend = %+v, server calls=%d", report, calls.Load())
	}
	assertSpendCost(t, report.Totals.ReportedCostUSD, price)
}

func TestHubSpendCountsMalformedPrimaryAndSuccessfulFallback(t *testing.T) {
	primaryPrice, fallbackPrice := 0.00004, 0.00002
	primary, primaryCalls := spendWireServer(t, &primaryPrice, true)
	fallback, fallbackCalls := spendWireServer(t, &fallbackPrice, false)
	h := spendHTTPHub(t, primary)
	ownModel(t, h, "spend-fallback", fallback.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Fallback = "spend-fallback" })
	var billed []*Response
	resp, err := h.Decide(context.Background(), testGate, yesNo(), WithBilling(func(_ context.Context, r *Response) { billed = append(billed, r) }))
	if err != nil || resp == nil || !resp.Fallback || resp.Instance != "spend-fallback" {
		t.Fatalf("fallback = %+v, %v", resp, err)
	}
	if len(billed) != 2 || billed[0].Instance != "DM1" || billed[1].Instance != "spend-fallback" || len(billed[0].Answers) != 0 {
		t.Fatalf("billed attempts = %+v", billed)
	}
	report := h.Spend(7)
	if primaryCalls.Load() != 1 || fallbackCalls.Load() != 1 || report.Totals.Calls != 2 || report.Totals.Failures != 1 || report.Totals.PrimaryCalls != 2 || report.Totals.InputTokens != 2000 {
		t.Fatalf("fallback spend = %+v", report)
	}
	assertSpendCost(t, report.Totals.CostUSD, primaryPrice+fallbackPrice)
}

func TestHubSpendCountsAsynchronousChallengerAndItsBilling(t *testing.T) {
	primaryPrice, challengerPrice := 0.00003, 0.00001
	primary, _ := spendWireServer(t, &primaryPrice, false)
	challenger, challengerCalls := spendWireServer(t, &challengerPrice, false)
	h := spendHTTPHub(t, primary)
	ownModel(t, h, "spend-challenger", challenger.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Challenger = "spend-challenger" })
	var wg sync.WaitGroup
	defer wg.Wait()
	var mu sync.Mutex
	billed := map[string]int{}
	resp, err := h.Decide(context.Background(), testGate, yesNo(),
		WithBackground(func(run func()) {
			wg.Add(1)
			go func() { defer wg.Done(); run() }()
		}),
		WithBilling(func(_ context.Context, r *Response) { mu.Lock(); billed[r.Instance]++; mu.Unlock() }),
	)
	if err != nil || resp == nil || resp.Instance != "DM1" {
		t.Fatalf("primary = %+v, %v", resp, err)
	}
	wg.Wait()
	report := h.Spend(7)
	if challengerCalls.Load() != 1 || report.Totals.Calls != 2 || report.Totals.PrimaryCalls != 1 || report.Totals.ChallengerCalls != 1 || report.Totals.InputTokens != 2000 || report.Totals.Failures != 0 {
		t.Fatalf("challenger spend = %+v", report)
	}
	mu.Lock()
	defer mu.Unlock()
	if billed["DM1"] != 1 || billed["spend-challenger"] != 1 {
		t.Fatalf("billed = %+v", billed)
	}
	assertSpendCost(t, report.Totals.CostUSD, primaryPrice+challengerPrice)
}

func TestHubSpendTestRoleRunsWithMasterSwitchOff(t *testing.T) {
	price := 0.000005
	srv, calls := spendWireServer(t, &price, false)
	h := spendHTTPHub(t, srv)
	cfg := h.Config()
	cfg.Enabled = false
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	resp, err := h.Test(context.Background(), "DM1")
	if err != nil || resp == nil {
		t.Fatalf("model test = %+v, %v", resp, err)
	}
	report := h.Spend(7)
	if calls.Load() != 1 || report.Totals.Calls != 1 || report.Totals.TestCalls != 1 || report.Totals.PrimaryCalls != 0 || report.Totals.ChallengerCalls != 0 {
		t.Fatalf("test role spend = %+v", report)
	}
	assertSpendCost(t, report.Totals.CostUSD, price)
}

func TestHubSpendDoesNotCountOffOrPreflightRejections(t *testing.T) {
	srv, calls := spendWireServer(t, nil, false)
	h := spendHTTPHub(t, srv)
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOff })
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrSiteOff) {
		t.Fatalf("off = %v", err)
	}
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn })
	if _, err := h.Decide(context.Background(), testGate, Request{}); err == nil {
		t.Fatal("invalid request accepted")
	}
	if _, err := h.Decide(context.Background(), testGate, Request{State: make(chan int), Questions: yesNo().Questions}); err == nil {
		t.Fatal("unserializable state accepted")
	}
	cfg := h.Config()
	cfg.Enabled = false
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Decide(context.Background(), testGate, yesNo()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled = %v", err)
	}
	if calls.Load() != 0 || h.Spend(7).Totals.Calls != 0 {
		t.Fatalf("preflight charged: server=%d report=%+v", calls.Load(), h.Spend(7))
	}
}

func TestHubSpendReportedZeroIsNotReplacedByMissingCostEstimate(t *testing.T) {
	zero := 0.0
	for _, tc := range []struct {
		name     string
		cost     *float64
		source   string
		expected float64
	}{
		{"reported zero", &zero, "reported", 0},
		{"missing cost", nil, "estimated", 1000 * jevInputPerMTok / 1e6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := spendWireServer(t, tc.cost, false)
			h := spendHTTPHub(t, srv)
			resp, err := h.Decide(context.Background(), testGate, yesNo())
			if err != nil || resp == nil || resp.Usage.CostSource != tc.source {
				t.Fatalf("usage source = %+v, %v", resp, err)
			}
			report := h.Spend(7)
			if report.Totals.Calls != 1 || report.Totals.UnknownCostCalls != 0 {
				t.Fatalf("spend = %+v", report)
			}
			assertSpendCost(t, report.Totals.CostUSD, tc.expected)
			assertSpendCost(t, resp.Usage.CostUSD, tc.expected)
			if tc.source == "reported" {
				assertSpendCost(t, report.Totals.EstimatedCostUSD, 0)
			} else {
				assertSpendCost(t, report.Totals.EstimatedCostUSD, tc.expected)
			}
		})
	}
}
