package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

func TestWorkspaceUsageExposesDecisionSpendWithoutRechargingTotals(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	now := time.Now().UTC()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "decider"), 0755); err != nil {
		t.Fatal(err)
	}
	stored := map[string]any{"version": 1, "startedAt": now.UnixMilli(), "updatedAt": now.UnixMilli(), "days": []map[string]any{{"day": now.Format(time.DateOnly), "provider": "openrouter", "model": "typesafe/jev-1.13", "calls": 1, "primaryCalls": 1, "inputTokens": 100, "costUSD": 0.0000042, "reportedCostUSD": 0.0000042}}}
	raw, _ := json.Marshal(stored)
	if err := os.WriteFile(filepath.Join(dir, "decider", "spend.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s.tun.SetDecider(decider.NewHub(decider.HubOptions{DataDir: dir, Now: func() time.Time { return now }}))
	for _, days := range []string{"7", "30", "90"} {
		rec := deciderRequest(t, s, http.MethodGet, "/api/usage?days="+days, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("usage %s: %d %s", days, rec.Code, rec.Body.String())
		}
		var body struct {
			DecisionSpend decider.SpendReport `json:"decisionSpend"`
			Totals        struct {
				CostUSD float64 `json:"costUSD"`
			} `json:"totals"`
			Cumulative struct {
				Days    int     `json:"days"`
				CostUSD float64 `json:"costUSD"`
			} `json:"cumulative"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.DecisionSpend.Days != body.Cumulative.Days || body.DecisionSpend.Totals.CostUSD != 0.0000042 || body.DecisionSpend.Today.Calls != 1 || len(body.DecisionSpend.Models) != 1 || body.DecisionSpend.Models[0].Provider != "openrouter" {
			t.Fatalf("decision spend missing from usage response: %+v", body)
		}
		if body.Totals.CostUSD != 0 || body.Cumulative.CostUSD != 0 {
			t.Fatal("application decision spend was added to workspace totals")
		}
	}
}
