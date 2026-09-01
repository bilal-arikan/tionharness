package agent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// guardedFailProvider fails every completion, carrying the tokens the failed
// turn already spent — what a provider does on a rate-limit or auth rejection
// that arrives after the request was sent.
type guardedFailProvider struct{ err error }

func (guardedFailProvider) Name() string { return "guarded-fail-test" }

func (p guardedFailProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return nil, p.err
}

var registerGuardedFailProvider sync.Once

func configureGuardedFailProvider(rt *Runtime, err error) {
	registerGuardedFailProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "guarded-fail-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) {
				return guardedFailProvider{err: guardedFailErr}, nil
			},
		))
	})
	guardedFailErr = err
	rt.providers.SetInstances([]providers.Instance{{ID: "guarded-fail-test", KindID: "guarded-fail-test"}})
}

// guardedFailErr is what the registered kind builds its provider with. The kind
// registry is process-global and registered once, so the error is swapped here
// rather than baked into the registration closure.
var guardedFailErr error

func guardedFailAgent(t *testing.T, rt *Runtime, name string) db.Agent {
	t.Helper()
	agent, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: name, Provider: "guarded-fail-test", Model: "opus",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func usageForAgent(t *testing.T, rt *Runtime, agentID string) db.Usage {
	t.Helper()
	rows, err := rt.db.UsageForDay(context.Background(), time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("usage for day: %v", err)
	}
	var got db.Usage
	for _, u := range rows {
		if u.AgentID == agentID {
			got = u
		}
	}
	return got
}

// The auxiliary funnel (reflect/summary/title/btw) must bill a turn that failed
// after the request was sent — recording usage only on the success path billed
// exactly the most expensive turns as zero.
func TestGuardedCompleteBillsFailedTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	configureGuardedFailProvider(rt, providers.WithUsage(
		errors.New("claude CLI usage/rate limit reached"),
		"opus",
		providers.Usage{InputTokens: 1234, OutputTokens: 7, CacheReadTokens: 500},
		3,
	))
	ctx := WithCallKind(context.Background(), KindReflect)
	agent := guardedFailAgent(t, rt, "Billed Aux")

	resp, err := rt.guardedComplete(ctx, agent, providers.Request{Model: "opus"}, false)
	if err == nil {
		t.Fatal("the failure must still propagate as an error")
	}
	if resp != nil {
		t.Fatalf("a failed turn must not return a response: %+v", resp)
	}

	got := usageForAgent(t, rt, agent.ID)
	if got.Calls == 0 {
		t.Fatal("failed auxiliary turn was not billed at all")
	}
	if got.InputTokens != 1234 || got.OutputTokens != 7 || got.CacheReadTokens != 500 {
		t.Fatalf("failed turn billed as %+v, want the tokens it actually spent", got)
	}
	if got.ProviderCalls != 3 {
		t.Fatalf("providerCalls = %d, want the failed turn's 3 internal round-trips", got.ProviderCalls)
	}
}

// A failure that never reached the provider (no usage attached) must not create
// a phantom usage record.
func TestGuardedCompleteDoesNotBillUsagelessFailure(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	configureGuardedFailProvider(rt, errors.New("binary not found"))
	ctx := WithCallKind(context.Background(), KindReflect)
	agent := guardedFailAgent(t, rt, "Unbilled Aux")

	if _, err := rt.guardedComplete(ctx, agent, providers.Request{Model: "opus"}, false); err == nil {
		t.Fatal("expected an error")
	}

	if got := usageForAgent(t, rt, agent.ID); got.Calls > 0 || got.InputTokens > 0 {
		t.Fatalf("a failure with no usage was billed: %+v", got)
	}
}
