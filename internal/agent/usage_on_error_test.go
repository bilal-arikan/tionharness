package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// failingProvider fails every completion, carrying the tokens the failed turn
// already spent — exactly what the claude-cli provider does on a rate-limit or
// auth rejection at the result envelope.
type failingProvider struct{ err error }

func (p failingProvider) Name() string { return "claude-cli" }

func (p failingProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return nil, p.err
}

// A turn that fails after the request was sent still paid for its input tokens.
// Recording usage only on the success path billed those turns as zero.
func TestRecordedCompleteBillsFailedTurn(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Billed", Provider: "claude-cli", Model: "opus"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	failure := providers.WithUsage(
		errors.New("claude CLI usage/rate limit reached"),
		"opus",
		providers.Usage{InputTokens: 4321, OutputTokens: 9, CacheReadTokens: 700},
		2,
	)
	provider := failingProvider{err: failure}

	resp, err := rt.recordedComplete(ctx, agent, provider, providers.Request{Model: "opus"})
	if err == nil {
		t.Fatal("the failure must still propagate as an error")
	}
	if resp != nil {
		t.Fatalf("a failed turn must not return a response: %+v", resp)
	}

	day := time.Now().Format("2006-01-02")
	rows, err := rt.db.UsageForDay(ctx, day)
	if err != nil {
		t.Fatalf("usage for day: %v", err)
	}
	var got db.Usage
	for _, u := range rows {
		if u.AgentID == agent.ID {
			got = u
		}
	}
	if got.InputTokens != 4321 || got.OutputTokens != 9 || got.CacheReadTokens != 700 {
		t.Fatalf("failed turn billed as %+v, want the tokens it actually spent", got)
	}
	if got.ProviderCalls != 2 {
		t.Fatalf("providerCalls = %d, want the failed turn's 2 internal round-trips", got.ProviderCalls)
	}
}

// A failure that never reached the provider (no usage attached) must not create
// a phantom usage record.
func TestRecordedCompleteDoesNotBillUsagelessFailure(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Unbilled", Provider: "claude-cli", Model: "opus"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := rt.recordedComplete(ctx, agent, failingProvider{err: errors.New("binary not found")}, providers.Request{Model: "opus"}); err == nil {
		t.Fatal("expected an error")
	}

	rows, err := rt.db.UsageForDay(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("usage for day: %v", err)
	}
	for _, u := range rows {
		if u.AgentID == agent.ID && (u.Calls > 0 || u.InputTokens > 0) {
			t.Fatalf("a failure with no usage was billed: %+v", u)
		}
	}
}
