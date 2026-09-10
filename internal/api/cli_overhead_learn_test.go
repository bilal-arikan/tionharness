package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestMeasuredPromptTokensPrefersFirstCallThenSingleCallUsage(t *testing.T) {
	if got := measuredPromptTokens(nil); got != 0 {
		t.Fatalf("nil response = %d, want 0", got)
	}
	first := &providers.Response{FirstCallPromptTokens: 31000, ProviderCalls: 4,
		Usage: providers.Usage{InputTokens: 4000, CacheReadTokens: 120000}}
	if got := measuredPromptTokens(first); got != 31000 {
		t.Fatalf("per-call figure ignored: %d", got)
	}
	single := &providers.Response{ProviderCalls: 1, Usage: providers.Usage{InputTokens: 1000, CacheReadTokens: 25000, CacheWriteTokens: 500}}
	if got := measuredPromptTokens(single); got != 26500 {
		t.Fatalf("single-call cumulative = %d, want 26500", got)
	}
	multi := &providers.Response{ProviderCalls: 3, Usage: providers.Usage{InputTokens: 3000, CacheReadTokens: 75000}}
	if got := measuredPromptTokens(multi); got != 0 {
		t.Fatalf("multi-call average must not be used as a sample, got %d", got)
	}
}

func TestLearnCLIOverheadFeedsProjectionAndFillers(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "cli", Provider: "claude-cli", Model: "opus"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	eager := countEagerTools(wsp.Runtime.ShippedToolCatalog(ctx, agentRow))

	// Nothing learned yet: the reference projection is the floor.
	tokens, source, samples := s.projectedCLIOverhead(ctx, wsp, agentRow, eager)
	if source != cliOverheadSourceReference || samples != 0 || tokens != conversation.PredictCLIOverhead(eager) {
		t.Fatalf("before learning = %d/%s/%d, want reference projection", tokens, source, samples)
	}

	// A turn estimated at 10k whose first call really carried 40k → 30k harness.
	resp := &providers.Response{FirstCallPromptTokens: 40000, ProviderCalls: 2}
	if _, ok := s.learnCLIOverhead(ctx, wsp, agentRow, 10000, resp); !ok {
		t.Fatal("turn was not learned")
	}
	tokens, source, samples = s.projectedCLIOverhead(ctx, wsp, agentRow, eager)
	if source != cliOverheadSourceMeasured || samples != 1 || tokens != 30000 {
		t.Fatalf("after one turn = %d/%s/%d, want 30000/measured/1", tokens, source, samples)
	}
	// Second turn: 20k estimate, 44k real → 24k; running mean 27k.
	if _, ok := s.learnCLIOverhead(ctx, wsp, agentRow, 20000, &providers.Response{FirstCallPromptTokens: 44000}); !ok {
		t.Fatal("second turn was not learned")
	}
	tokens, _, samples = s.projectedCLIOverhead(ctx, wsp, agentRow, eager)
	if tokens != 27000 || samples != 2 {
		t.Fatalf("mean after two turns = %d/%d, want 27000/2", tokens, samples)
	}

	// Over-counting heuristic (real < estimate) clamps to 0 but still samples.
	if _, ok := s.learnCLIOverhead(ctx, wsp, agentRow, 50000, &providers.Response{FirstCallPromptTokens: 30000}); !ok {
		t.Fatal("clamped turn was not learned")
	}
	if tokens, _, samples = s.projectedCLIOverhead(ctx, wsp, agentRow, eager); tokens != 18000 || samples != 3 {
		t.Fatalf("mean after clamp = %d/%d, want 18000/3", tokens, samples)
	}

	// The learned figure is what the context meter (and hence the fold gate)
	// budgets under the cli-harness bucket, marked as calibrated.
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agentRow.ID, Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var harness *contextFiller
	for _, f := range s.systemFillers(ctx, wsp, session, nil, false) {
		if f.Role == "cli-harness" {
			ff := f
			harness = &ff
		}
	}
	if harness == nil {
		t.Fatal("no cli-harness bucket for a claude-cli agent")
	}
	if harness.Tokens != 18000 || !harness.Calibrated {
		t.Fatalf("harness bucket = %+v, want 18000 calibrated", *harness)
	}

	// The preview reports the same figure and labels its source.
	preview := computeCLIOverhead(ctx, wsp, agentRow.Provider, "", 1000, eager)
	s.applyLearnedCLIOverhead(ctx, wsp, agentRow, preview)
	if preview.PredictedOverhead != 18000 || preview.PredictedSource != cliOverheadSourceMeasured || preview.PredictedSamples != 3 {
		t.Fatalf("preview = %d/%s/%d", preview.PredictedOverhead, preview.PredictedSource, preview.PredictedSamples)
	}
}

func TestLearnCLIOverheadSkipsNonCLIAndUnmeasurableTurns(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	native, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "native", Provider: "anthropic", Model: "claude-sonnet-5"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, ok := s.learnCLIOverhead(ctx, wsp, native, 1000, &providers.Response{FirstCallPromptTokens: 5000}); ok {
		t.Fatal("native provider must not learn a CLI harness")
	}
	cli, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "cli", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, ok := s.learnCLIOverhead(ctx, wsp, cli, 0, &providers.Response{FirstCallPromptTokens: 5000}); ok {
		t.Fatal("a turn without a comparable estimate must not learn")
	}
	if _, ok := s.learnCLIOverhead(ctx, wsp, cli, 1000, &providers.Response{ProviderCalls: 3, Usage: providers.Usage{InputTokens: 9000}}); ok {
		t.Fatal("a multi-call turn without a per-call figure must not learn")
	}
	if _, ok := s.learnedCLIOverhead(ctx, wsp, cli); ok {
		t.Fatal("nothing should have been stored")
	}
	// Reference floor still shows in the meter before any learning.
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", AgentID: cli.ID, Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for _, f := range s.systemFillers(ctx, wsp, session, nil, false) {
		if f.Role == "cli-harness" {
			if f.Calibrated || f.Tokens < conversation.CLIBaseTokens {
				t.Fatalf("reference harness bucket = %+v", f)
			}
			return
		}
	}
	t.Fatal("no cli-harness bucket before learning")
}
