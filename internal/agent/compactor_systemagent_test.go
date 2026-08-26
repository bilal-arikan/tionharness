package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const embeddedSummaryPrompt = "You summarize structured workspace data for the user. Reply in the same language as the data. Be concise and well structured: a one-line overview followed by short grouped markdown bullets. Do not invent items that are not present in the data."

type compactorTestProvider struct {
	request providers.Request
}

func (p *compactorTestProvider) Name() string { return "compactor-test" }

func (p *compactorTestProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.request = req
	return &providers.Response{Text: "summary produced", Model: req.Model}, nil
}

var (
	registerCompactorTestProvider sync.Once
	activeCompactorTestProvider   *compactorTestProvider
)

func configureCompactorTestProvider(rt *Runtime) *compactorTestProvider {
	registerCompactorTestProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "compactor-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return activeCompactorTestProvider, nil },
		))
	})
	provider := &compactorTestProvider{}
	activeCompactorTestProvider = provider
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{ID: "compactor-test-instance", KindID: "compactor-test"}})
	rt.tun = NewTunables()
	return provider
}

func TestResolveCompactorConfigDefaultPathUnchanged(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)

	got, prompt := rt.resolveCompactorConfig(db.Agent{Model: "session-model"})

	if prompt != embeddedSummaryPrompt {
		t.Fatalf("default compactor prompt = %q, want fixed embedded prompt", prompt)
	}
	if got.Model != "haiku" {
		t.Fatalf("default compactor model = %q, want %q", got.Model, "haiku")
	}
}

func TestResolveCompactorConfigUsesEnabledWorkspaceAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Custom Compactor", System: true, SystemKey: "compactor",
		Soul: "custom summary prompt", Model: "custom-summary-model",
	}); err != nil {
		t.Fatalf("create system agent: %v", err)
	}

	got, prompt := rt.resolveCompactorConfig(db.Agent{Provider: "calling-provider", Model: "session-model"})

	if prompt != "custom summary prompt" {
		t.Fatalf("summary prompt = %q, want custom prompt", prompt)
	}
	if got.Model != "custom-summary-model" {
		t.Fatalf("summary model = %q, want custom model", got.Model)
	}
	if got.Provider != "calling-provider" {
		t.Fatalf("summary provider = %q, want calling provider", got.Provider)
	}
}

func TestSummarizeDisabledCompactorRunsEmbeddedFallback(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	provider := configureCompactorTestProvider(rt)
	callingAgent, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Caller", Provider: "compactor-test", ProviderInstanceID: "compactor-test-instance", Model: "session-model",
	})
	if err != nil {
		t.Fatalf("create calling agent: %v", err)
	}
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Disabled Compactor", System: true, SystemKey: "compactor",
		Soul: "disabled summary prompt", Model: "disabled-summary-model", Disabled: true,
	}); err != nil {
		t.Fatalf("create system agent: %v", err)
	}
	if _, err := rt.db.CreateTask(context.Background(), db.Task{Title: "Fallback coverage"}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := rt.Summarize(context.Background(), callingAgent.ID, SummaryBoard)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got == "" {
		t.Fatal("Summarize returned empty fallback result")
	}
	if provider.request.System == "" {
		t.Fatal("fallback provider request has empty prompt")
	}
	if provider.request.Model == "" {
		t.Fatal("fallback provider request has empty model")
	}
	if provider.request.System != embeddedSummaryPrompt {
		t.Fatalf("fallback prompt = %q, want fixed embedded prompt", provider.request.System)
	}
	if provider.request.Model != "haiku" {
		t.Fatalf("fallback model = %q, want %q", provider.request.Model, "haiku")
	}
}

func TestSummarizeCompactorResolveErrorRunsEmbeddedFallback(t *testing.T) {
	originalDefaults := systemAgentDefaults
	withoutCompactor := make([]SystemAgentDefinition, 0, len(originalDefaults)-1)
	for _, def := range originalDefaults {
		if def.SystemKey != "compactor" {
			withoutCompactor = append(withoutCompactor, def)
		}
	}
	systemAgentDefaults = withoutCompactor
	t.Cleanup(func() { systemAgentDefaults = originalDefaults })

	rt := newSystemAgentResolveRuntime(t)
	provider := configureCompactorTestProvider(rt)
	rt.tun.SetTitleModel("haiku")
	callingAgent, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Caller", Provider: "compactor-test", ProviderInstanceID: "compactor-test-instance", Model: "session-model",
	})
	if err != nil {
		t.Fatalf("create calling agent: %v", err)
	}
	if _, err := rt.db.CreateTask(context.Background(), db.Task{Title: "Resolve error fallback coverage"}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := rt.Summarize(context.Background(), callingAgent.ID, SummaryBoard)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got != "summary produced" {
		t.Fatalf("Summarize = %q, want %q", got, "summary produced")
	}
	if provider.request.System != embeddedSummaryPrompt {
		t.Fatalf("fallback prompt = %q, want fixed embedded prompt", provider.request.System)
	}
	if provider.request.Model != "haiku" {
		t.Fatalf("fallback model = %q, want %q", provider.request.Model, "haiku")
	}
}
