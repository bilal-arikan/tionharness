package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const embeddedTitlePrompt = "You generate short titles. Given a user's request, message, or conversation, reply with a concise title of 3 to 6 words that summarizes it. Rules: reply with ONLY the title — no surrounding quotes, no trailing punctuation, no markdown, no preamble. Maximum 60 characters. Write the title in the same language as the input."

type titleTestProvider struct{}

func (*titleTestProvider) Name() string { return "title-test" }

func (*titleTestProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	return &providers.Response{Text: `{"title":"Generated Test Title"}`}, nil
}

var registerTitleTestProvider sync.Once

func configureTitleTestProvider(rt *Runtime) {
	registerTitleTestProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "title-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return &titleTestProvider{}, nil },
		))
	})
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{ID: "title-test-instance", KindID: "title-test"}})
	rt.tun = NewTunables()
}

func TestResolveTitleConfigDefaultPathUnchanged(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)

	got, prompt := rt.resolveTitleConfig(db.Agent{Model: "session-model"})

	if prompt == "" {
		t.Fatal("default titler prompt is empty")
	}
	if prompt != embeddedTitlePrompt {
		t.Fatal("default titler prompt differs from embedded title prompt")
	}
	if got.Model != "haiku" {
		t.Fatalf("default titler model = %q, want %q", got.Model, "haiku")
	}
}

func TestResolveTitleConfigUsesCustomizedTitler(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name:      "Custom Titler",
		System:    true,
		SystemKey: "titler",
		Soul:      "custom title prompt",
		Model:     "custom-title-model",
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, prompt := rt.resolveTitleConfig(db.Agent{Model: "session-model"})

	if prompt != "custom title prompt" {
		t.Fatalf("title prompt = %q, want custom prompt", prompt)
	}
	if got.Model != "custom-title-model" {
		t.Fatalf("title model = %q, want custom model", got.Model)
	}
}

func TestResolveTitleConfigDisabledTitlerFallsBack(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	configureTitleTestProvider(rt)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name:      "Disabled Titler",
		System:    true,
		SystemKey: "titler",
		Soul:      "disabled title prompt",
		Model:     "disabled-title-model",
		Disabled:  true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	title, err := rt.GenerateTitle(context.Background(), db.Agent{
		Provider:           "title-test",
		ProviderInstanceID: "title-test-instance",
		Model:              "session-model",
	}, "Strengthen disabled titler coverage")
	if err != nil {
		t.Fatalf("GenerateTitle: %v", err)
	}
	if strings.TrimSpace(title) == "" {
		t.Fatal("GenerateTitle returned an empty title for disabled titler")
	}
}
