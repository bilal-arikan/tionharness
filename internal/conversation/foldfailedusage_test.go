package conversation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// failingUsageProvider fails the way a CLI provider fails after the prompt was
// already accepted and paid for: an error carrying the usage the attempt spent.
type failingUsageProvider struct {
	usage providers.Usage
	model string
	calls int
}

func (p *failingUsageProvider) Name() string { return "failing" }
func (p *failingUsageProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	return nil, providers.WithUsage(errors.New("provider died mid-fold"), p.model, p.usage, p.calls)
}

// bareErrorProvider fails without telling anyone what it spent.
type bareErrorProvider struct{}

func (bareErrorProvider) Name() string { return "bare" }
func (bareErrorProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	return nil, errors.New("provider died before it billed anything")
}

func foldUsageTestDB(t *testing.T) (*db.DB, db.Agent) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	agent, err := d.CreateAgent(context.Background(), db.Agent{Name: "A", Provider: "anthropic", Model: "agent-model"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return d, agent
}

func compactBucket(t *testing.T, d *db.DB, agentID string) db.KindStat {
	t.Helper()
	u, err := d.GetUsageToday(context.Background(), agentID)
	if err != nil {
		t.Fatalf("usage today: %v", err)
	}
	return u.ByKind[db.UsageKindCompact]
}

// TestFoldBillsTheAttemptThatFailed pins the money side of a failed fold. Both
// fold entry points call provider.Complete directly — outside guardedComplete and
// outside the tool loop — so nothing else in the stack bills them. A fold sends
// the entire pending transcript, so an attempt that dies after the prompt landed
// is the single most expensive request of the session; returning the error bare
// made it cost zero on the books.
func TestFoldBillsTheAttemptThatFailed(t *testing.T) {
	spent := providers.Usage{InputTokens: 900, CacheReadTokens: 400, OutputTokens: 0}

	t.Run("rolling fold", func(t *testing.T) {
		d, agent := foldUsageTestDB(t)
		p := &failingUsageProvider{usage: spent, model: "fold-model", calls: 2}

		if _, err := summarizeRendered(context.Background(), d, p, agent, "", "u: hi\na: there\n"); err == nil {
			t.Fatal("summarizeRendered must return the provider error")
		}

		got := compactBucket(t, d, agent.ID)
		if got.Calls != 2 || got.InputTokens != 900 || got.CacheReadTokens != 400 {
			t.Fatalf("compact bucket = %+v, want 2 calls / 900 in / 400 cache-read", got)
		}
	})

	t.Run("handoff", func(t *testing.T) {
		d, agent := foldUsageTestDB(t)
		p := &failingUsageProvider{usage: spent, model: "fold-model", calls: 1}

		if _, err := BuildHandoff(context.Background(), d, p, agent, "", "u: hi\n", HandoffEnv{}, ""); err == nil {
			t.Fatal("BuildHandoff must return the provider error")
		}

		got := compactBucket(t, d, agent.ID)
		if got.Calls != 1 || got.InputTokens != 900 {
			t.Fatalf("compact bucket = %+v, want 1 call / 900 in", got)
		}
	})
}

// TestFoldDoesNotInventUsageForAnUnbilledFailure is the other half of the
// contract: only an error CARRYING usage is billed. A provider that died without
// reporting what it spent gets no guessed number — a fabricated charge is worse
// than the gap it fills.
func TestFoldDoesNotInventUsageForAnUnbilledFailure(t *testing.T) {
	d, agent := foldUsageTestDB(t)

	if _, err := summarizeRendered(context.Background(), d, bareErrorProvider{}, agent, "", "u: hi\n"); err == nil {
		t.Fatal("summarizeRendered must return the provider error")
	}

	if got := compactBucket(t, d, agent.ID); got.Calls != 0 || got.InputTokens != 0 {
		t.Fatalf("compact bucket = %+v, want nothing billed", got)
	}
}

// TestFailedFoldIsBilledUnderTheModelTheProviderReports keeps the cost attached
// to the model that actually ran. A CLI provider can serve a different model than
// the agent asked for, and the error carries the one it used; billing the agent's
// requested model would price the failure wrong.
func TestFailedFoldIsBilledUnderTheModelTheProviderReports(t *testing.T) {
	d, agent := foldUsageTestDB(t)
	p := &failingUsageProvider{usage: providers.Usage{InputTokens: 10}, model: "served-model", calls: 1}

	if _, err := summarizeRendered(context.Background(), d, p, agent, "", "u: hi\n"); err == nil {
		t.Fatal("summarizeRendered must return the provider error")
	}

	u, err := d.GetUsageToday(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("usage today: %v", err)
	}
	if m := u.ByModel[db.ModelKey("anthropic", "served-model")]; m.InputTokens != 10 {
		t.Fatalf("served-model bucket = %+v, want 10 input tokens", m)
	}
	if m := u.ByModel[db.ModelKey("anthropic", "agent-model")]; m.InputTokens != 0 {
		t.Fatalf("agent-model bucket = %+v, want nothing", m)
	}
}
