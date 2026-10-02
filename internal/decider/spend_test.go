package decider

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func spendTestTime() time.Time { return time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC) }
func spendTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func recordSpend(t *testing.T, store *spendStore, at time.Time, authority, role, provider, model string, usage Usage, failed bool) {
	t.Helper()
	if err := store.record(at, authority, role, provider, model, usage, failed); err != nil {
		t.Fatal(err)
	}
}

func TestSpendSourcesRolesAndMicroCosts(t *testing.T) {
	at := spendTestTime()
	store := openSpendStore("", spendTestLogger(), at)
	recordSpend(t, store, at, "setup", "", "openrouter", "jev", Usage{InputTokens: 10, OutputTokens: 2, CostSource: "reported"}, false)
	recordSpend(t, store, at, "setup", "primary", "openrouter", "jev", Usage{InputTokens: 20, OutputTokens: 3, CostUSD: 0.0000021, CostSource: "reported"}, false)
	recordSpend(t, store, at, "worker-review", "challenger", "openrouter", "jev", Usage{InputTokens: 30, OutputTokens: 4, CostUSD: 0.0000037, CostSource: "estimated"}, false)
	recordSpend(t, store, at, "test", "test", "local", "jev-local", Usage{InputTokens: 40, OutputTokens: 5, CostSource: "local"}, false)
	recordSpend(t, store, at, "setup", "fallback", "openrouter", "jev", Usage{}, true)
	recordSpend(t, store, at, "setup", "primary", "openrouter", "jev", Usage{CostUSD: 0.0000004, CostSource: "reported"}, true)
	report := store.report(at, 7)
	if report.Totals.Calls != 6 || report.Totals.Failures != 2 || report.Totals.UnknownCostCalls != 1 || report.Totals.PrimaryCalls != 4 || report.Totals.ChallengerCalls != 1 || report.Totals.TestCalls != 1 {
		t.Fatalf("source/role accounting failed: %+v", report.Totals)
	}
	if report.Totals.InputTokens != 100 || report.Totals.OutputTokens != 14 || math.Abs(report.Totals.CostUSD-0.0000062) > 1e-15 || math.Abs(report.Totals.ReportedCostUSD-0.0000025) > 1e-15 || math.Abs(report.Totals.EstimatedCostUSD-0.0000037) > 1e-15 {
		t.Fatalf("microcosts or token counters were lost: %+v", report.Totals)
	}
	if len(report.Models) != 2 || report.Models[0].Provider != "openrouter" || report.Models[0].Calls != 5 || report.Models[1].UnknownCostCalls != 0 || report.Today != report.Totals {
		t.Fatalf("model/today accounting failed: %+v", report)
	}
}

func TestSpendConcurrentWritesAndRestart(t *testing.T) {
	dir, at := t.TempDir(), spendTestTime()
	store := openSpendStore(dir, spendTestLogger(), at)
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			role := "primary"
			if i%3 == 0 {
				role = "challenger"
			}
			if err := store.record(at.Add(time.Second), "session-setup", role, "openrouter", "jev", Usage{InputTokens: 10, OutputTokens: 1, CostUSD: 0.0000001, CostSource: "reported"}, i%10 == 0); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	reopened := openSpendStore(dir, spendTestLogger(), at.Add(time.Hour))
	report := reopened.report(at.Add(time.Hour), 1)
	if report.StorageError || report.Totals.Calls != 60 || report.Totals.Failures != 6 || report.Totals.InputTokens != 600 || report.Totals.OutputTokens != 60 || report.Totals.ChallengerCalls != 20 || report.Totals.PrimaryCalls != 40 {
		t.Fatalf("concurrent durable counters lost updates: %+v", report)
	}
	if report.StartedAt != at.UnixMilli() || report.UpdatedAt != at.Add(time.Second).UnixMilli() || math.Abs(report.Totals.CostUSD-0.000006) > 1e-15 {
		t.Fatalf("restart changed coverage timestamps or cost: %+v", report)
	}
}

func TestSpendReportCalendarWindowsAndRetention(t *testing.T) {
	dir, at := t.TempDir(), spendTestTime()
	store := openSpendStore(dir, spendTestLogger(), at)
	known := Usage{CostSource: "reported", CostUSD: 1}
	for _, age := range []int{365, 364, 90, 89, 7, 6, 1, 0} {
		recordSpend(t, store, at.AddDate(0, 0, -age), "setup", "primary", "openrouter", "jev", known, false)
	}
	if report := store.report(at, 1); report.Totals.Calls != 1 || report.Today.Calls != 1 {
		t.Fatalf("one-day window: %+v", report)
	}
	if report := store.report(at, 7); report.Totals.Calls != 3 || report.Totals.CostUSD != 3 || report.Today.Calls != 1 {
		t.Fatalf("inclusive seven-day window: %+v", report)
	}
	if report := store.report(at, 365); report.Days != 90 || report.Totals.Calls != 5 {
		t.Fatalf("ninety-day public cap: %+v", report)
	}
	if report := store.report(at, 0); report.Days != 30 {
		t.Fatalf("default window: %+v", report)
	}
	file := spendFile{}
	data, err := os.ReadFile(filepath.Join(dir, spendFileName))
	if err != nil || json.Unmarshal(data, &file) != nil || len(file.Days) != 7 {
		t.Fatalf("365-day retention did not prune the oldest day: rows=%d err=%v", len(file.Days), err)
	}
	// The public window follows UTC dates, independent of the caller's timezone.
	localNow := time.Date(2026, 10, 3, 1, 0, 0, 0, time.FixedZone("Istanbul", 3*60*60))
	if report := store.report(localNow, 1); report.Today.Calls != 1 {
		t.Fatalf("timezone changed the UTC reporting day: %+v", report)
	}
	// Future records do not leak into a report for an earlier day.
	recordSpend(t, store, at.AddDate(0, 0, 1), "setup", "primary", "openrouter", "jev", known, false)
	if report := store.report(at, 7); report.Totals.Calls != 3 {
		t.Fatalf("future row leaked into earlier report: %+v", report)
	}
}

func TestSpendModelIdentityAndReturnedDataAreIndependent(t *testing.T) {
	at := spendTestTime()
	store := openSpendStore("", spendTestLogger(), at)
	for _, provider := range []string{"first-provider", "second-provider"} {
		recordSpend(t, store, at, "different-workspace-authority", "primary", provider, "same-model", Usage{CostSource: "reported"}, false)
	}
	recordSpend(t, store, at, "other-workspace-authority", "test", "first-provider", "same-model", Usage{CostSource: "reported"}, false)
	report := store.report(at, 1)
	if len(report.Models) != 2 || report.Totals.Calls != 3 || report.Models[0].Calls != 2 {
		t.Fatalf("provider identity or workspace aggregation was lost: %+v", report)
	}
	report.Models[0].Calls = 1000
	report.Models[0].Provider = "mutated"
	report.Totals.Calls = 1000
	fresh := store.report(at, 1)
	if fresh.Totals.Calls != 3 || fresh.Models[0].Calls != 2 || fresh.Models[0].Provider == "mutated" {
		t.Fatal("report exposed mutable internal data")
	}
}

func TestSpendSurvivesLedgerReplacementAndContainsNoAuthorityPayload(t *testing.T) {
	dir, at := t.TempDir(), spendTestTime()
	store := openSpendStore(dir, spendTestLogger(), at)
	recordSpend(t, store, at, "never-persist-this-authority-value", "primary", "openrouter", "jev", Usage{CostSource: "estimated", CostUSD: 0.0000002}, false)
	// Rotation or deletion of both ledger segments cannot affect daily rollups.
	for _, name := range []string{"ledger.jsonl", "ledger.1.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("replaced ledger content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reopened := openSpendStore(dir, spendTestLogger(), at.Add(time.Hour))
	if report := reopened.report(at, 1); report.Totals.Calls != 1 || report.Totals.EstimatedCostUSD != 0.0000002 || report.StorageError {
		t.Fatalf("ledger replacement altered daily rollups: %+v", report)
	}
	data, err := os.ReadFile(filepath.Join(dir, spendFileName))
	if err != nil || strings.Contains(string(data), "never-persist-this-authority-value") || strings.Contains(string(data), "ledger") {
		t.Fatalf("spend file included unrelated payload: %s, %v", data, err)
	}
}

func TestSpendCorruptHistoryIsFlaggedAndPreserved(t *testing.T) {
	for _, content := range []string{"{broken", `{"version":9}`, `{"version":1}`, `{"version":1,"startedAt":1,"updatedAt":1,"days":[{"day":"not-a-date"}]}`, `{"version":1,"startedAt":1,"updatedAt":1,"days":[{"day":"2026-10-02","provider":"p","model":"m","calls":-1}]}`} {
		t.Run(content, func(t *testing.T) {
			dir, at := t.TempDir(), spendTestTime()
			path := filepath.Join(dir, spendFileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			store := openSpendStore(dir, spendTestLogger(), at)
			if report := store.report(at, 1); !report.StorageError {
				t.Fatal("corrupt history silently appeared complete")
			}
			if err := store.record(at, "setup", "primary", "openrouter", "jev", Usage{}, true); err == nil {
				t.Fatal("record did not report blocked durable storage")
			}
			if report := store.report(at, 1); !report.StorageError || report.Totals.Calls != 1 || report.Totals.UnknownCostCalls != 1 {
				t.Fatalf("current-process counts or incomplete flag were lost: %+v", report)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != content {
				t.Fatal("corrupt durable history was overwritten")
			}
		})
	}
}

func TestSpendWriteFailureRecoversWithoutLosingCurrentProcessCalls(t *testing.T) {
	dir, at := t.TempDir(), spendTestTime()
	store := openSpendStore(dir, spendTestLogger(), at)
	known := Usage{CostSource: "reported", CostUSD: 0.000001}
	recordSpend(t, store, at, "setup", "primary", "openrouter", "jev", known, false)
	tmp := filepath.Join(dir, spendFileName+".tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.record(at, "setup", "primary", "openrouter", "jev", known, false); err == nil {
		t.Fatal("blocked atomic write unexpectedly succeeded")
	}
	if report := store.report(at, 1); !report.StorageError || report.Totals.Calls != 2 {
		t.Fatalf("write failure hid the real call: %+v", report)
	}
	if err := os.Remove(tmp); err != nil {
		t.Fatal(err)
	}
	recordSpend(t, store, at, "setup", "primary", "openrouter", "jev", known, false)
	reopened := openSpendStore(dir, spendTestLogger(), at.Add(time.Hour))
	if report := reopened.report(at, 1); report.StorageError || report.Totals.Calls != 3 {
		t.Fatalf("recovered persistence lost calls: %+v", report)
	}
}

func TestSpendInvalidPricesStayUnknownInsteadOfFree(t *testing.T) {
	at := spendTestTime()
	store := openSpendStore(t.TempDir(), spendTestLogger(), at)
	for _, cost := range []float64{math.NaN(), math.Inf(1), -1} {
		recordSpend(t, store, at, "setup", "primary", "openrouter", "jev", Usage{CostSource: "reported", CostUSD: cost, InputTokens: -3, OutputTokens: -1}, false)
	}
	recordSpend(t, store, at, "setup", "primary", "", "", Usage{CostSource: "unknown"}, true)
	if report := store.report(at, 1); report.Totals.UnknownCostCalls != 4 || report.Totals.CostUSD != 0 || report.Totals.InputTokens != 0 || report.Totals.OutputTokens != 0 || report.StorageError {
		t.Fatalf("invalid price treatment: %+v", report)
	}
}
