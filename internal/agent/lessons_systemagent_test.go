package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const embeddedLessonPrompt = "You review a failed AI-agent turn and distill ONE reusable lesson for future turns.\nReply with 1-3 plain sentences: what failed, the likely root cause, and how to avoid it next time.\nGeneralize (a rule the agent can apply again), do not just restate the error.\nIf the failure is not generalizable (one-off cancellation, external outage, missing login), reply with exactly: NONE"

const embeddedInsightPrompt = "You are a retrospective analyst for the TionHarness multi-agent runtime.\nYou are given ONE lens instruction and ONE past session's evidence (error steps + debug events).\nApply the lens strictly and emit high-signal findings only — no speculation, no restating raw errors.\nPrefer recurring, generalizable problems. If nothing qualifies, return an empty findings array."

type analysisTestProvider struct {
	mu      sync.Mutex
	request providers.Request
	err     error
	calls   int
}

func (*analysisTestProvider) Name() string { return "analysis-test" }

func (p *analysisTestProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.request = req
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return &providers.Response{Text: `{"findings":[]}`}, nil
}

func (p *analysisTestProvider) captured() providers.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request
}

func TestResolveInsightConfigKeepsCallerModelForIncompatibleProvider(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	got, _, err := rt.resolveInsightConfig(db.Agent{Provider: "codex-cli", Model: "gpt-5"})
	if err != nil {
		t.Fatalf("resolve insight config: %v", err)
	}
	if got.Model != "gpt-5" {
		t.Fatalf("resolved model = %q, want caller model %q", got.Model, "gpt-5")
	}
}

func TestInsightScanStopsAfterPermanentProviderFailure(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	provider.err = providers.ErrPermanentProviderFailure
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Permanent failure caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model",
	})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	for i := 0; i < 2; i++ {
		session, createErr := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Failure evidence"})
		if createErr != nil {
			t.Fatalf("create session: %v", createErr)
		}
		if _, addErr := rt.db.AddMessage(ctx, db.Message{
			SessionID: session.ID, Role: "assistant", Text: "failed",
			Steps: `[{"kind":"error","reason":"provider_error","text":"boom"}]`,
		}); addErr != nil {
			t.Fatalf("add evidence: %v", addErr)
		}
	}

	_, err = rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}, Concurrency: 1}, agent.ID)
	if !errors.Is(err, providers.ErrPermanentProviderFailure) {
		t.Fatalf("RunInsightScan error = %v, want permanent provider failure", err)
	}
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	sessions, listErr := rt.db.ListSessions(ctx, agent.ID)
	if listErr != nil {
		t.Fatalf("list sessions: %v", listErr)
	}
	var failed db.Session
	for _, session := range sessions {
		if session.Kind == db.SessionKindInsight {
			failed = session
			break
		}
	}
	if failed.ID == "" {
		t.Fatal("permanent provider failure did not create an insight session")
	}
	if !strings.Contains(failed.Title, "başarısız") || !strings.Contains(failed.Title, providers.ErrPermanentProviderFailure.Error()) {
		t.Fatalf("failed scan title = %q, want failure and provider error", failed.Title)
	}
	if failed.RunState != turnStatusFailed {
		t.Fatalf("failed scan RunState = %q, want %q", failed.RunState, turnStatusFailed)
	}
	messages, messageErr := rt.db.ListMessages(ctx, failed.ID)
	if messageErr != nil {
		t.Fatalf("list failed scan messages: %v", messageErr)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Text, providers.ErrPermanentProviderFailure.Error()) {
		t.Fatalf("failed scan transcript = %#v, want provider error", messages)
	}
}

var (
	registerAnalysisTestProvider sync.Once
	analysisProviderMu           sync.Mutex
	activeAnalysisProvider       *analysisTestProvider
)

func configureAnalysisTestProvider(rt *Runtime) *analysisTestProvider {
	registerAnalysisTestProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "analysis-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) {
				analysisProviderMu.Lock()
				defer analysisProviderMu.Unlock()
				return activeAnalysisProvider, nil
			},
		))
	})
	p := &analysisTestProvider{}
	analysisProviderMu.Lock()
	activeAnalysisProvider = p
	analysisProviderMu.Unlock()
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{ID: "analysis-test-instance", KindID: "analysis-test"}})
	return p
}

func TestSystemAgentProductionCallsRecordSystemKeyUsageKinds(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	configureAnalysisTestProvider(rt)
	rt.tun = NewTunables()
	ctx := context.Background()
	callingAgent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Usage caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model",
	})
	if err != nil {
		t.Fatalf("create calling agent: %v", err)
	}

	if _, err := rt.GenerateTitle(ctx, callingAgent, "System usage taxonomy"); err != nil {
		t.Fatalf("GenerateTitle: %v", err)
	}
	if _, err := rt.db.CreateTask(ctx, db.Task{Title: "Usage taxonomy task"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := rt.Summarize(ctx, callingAgent.ID, SummaryBoard); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: callingAgent.ID, Title: "Usage evidence"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	rt.reflectLessons(ctx, session.ID, []lessonEvidence{{tool: "Bash", input: "false", errs: "exit status 1"}}, "")
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID: session.ID, Role: "assistant", Text: "failed",
		Steps: `[{"kind":"error","reason":"provider_error","text":"boom"}]`,
	}); err != nil {
		t.Fatalf("add insight evidence: %v", err)
	}
	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}}, callingAgent.ID); err != nil {
		t.Fatalf("RunInsightScan: %v", err)
	}

	usage, err := rt.db.GetUsageToday(ctx, callingAgent.ID)
	if err != nil {
		t.Fatalf("GetUsageToday: %v", err)
	}
	for _, kind := range []string{
		"system:titler:title",
		"system:overview-summarizer:summary",
		"system:lesson-extractor:reflect",
		"system:insight:reflect",
	} {
		if got := usage.ByKind[kind].Calls; got != 1 {
			t.Errorf("usage.ByKind[%q].Calls = %d, want 1; all kinds: %#v", kind, got, usage.ByKind)
		}
	}
}

func TestResolveLessonConfigDefaultPathUnchanged(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	got, prompt, err := rt.resolveLessonConfig(db.Agent{Model: "caller-model"})
	if err != nil {
		t.Fatalf("resolve lesson config: %v", err)
	}
	if prompt != embeddedLessonPrompt {
		t.Fatal("default lesson prompt differs from fixed embedded prompt")
	}
	if got.Model != "haiku" {
		t.Fatalf("default lesson model = %q, want haiku", got.Model)
	}
}

func TestResolveInsightConfigDefaultPathUnchanged(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	got, prompt, err := rt.resolveInsightConfig(db.Agent{Model: "caller-model"})
	if err != nil {
		t.Fatalf("resolve insight config: %v", err)
	}
	if prompt != embeddedInsightPrompt {
		t.Fatal("default insight prompt differs from fixed embedded prompt")
	}
	if got.Model != "haiku" {
		t.Fatalf("default insight model = %q, want haiku", got.Model)
	}
}

func TestResolveLessonConfigUsesEnabledWorkspaceAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Custom Lesson Extractor", System: true, SystemKey: "lesson-extractor",
		Soul: "custom lesson prompt", Model: "custom-lesson-model",
	}); err != nil {
		t.Fatalf("create system agent: %v", err)
	}

	got, prompt, err := rt.resolveLessonConfig(db.Agent{Provider: "calling-provider", Model: "caller-model"})
	if err != nil {
		t.Fatalf("resolve lesson config: %v", err)
	}
	if prompt != "custom lesson prompt" {
		t.Fatalf("lesson prompt = %q, want custom prompt", prompt)
	}
	if got.Model != "custom-lesson-model" {
		t.Fatalf("lesson model = %q, want custom model", got.Model)
	}
}

func TestResolveLessonConfigKeepsCallingProvider(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Custom Lesson Extractor", System: true, SystemKey: "lesson-extractor",
		Soul: "custom lesson prompt", Model: "custom-lesson-model", Provider: "system-provider",
	}); err != nil {
		t.Fatalf("create system agent: %v", err)
	}

	got, _, err := rt.resolveLessonConfig(db.Agent{Provider: "calling-provider", Model: "caller-model"})
	if err != nil {
		t.Fatalf("resolve lesson config: %v", err)
	}
	if got.Provider != "calling-provider" {
		t.Fatalf("lesson provider = %q, want calling provider", got.Provider)
	}
}

func TestLessonsResolveSystemAgentUnknownKeyReturnsError(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)

	if _, _, err := rt.ResolveSystemAgent("bilinmeyen-anahtar"); err == nil {
		t.Fatal("ResolveSystemAgent returned nil error for unknown key")
	}
}

func TestResolveAnalysisSystemAgentErrorReturnsEmbeddedPrompt(t *testing.T) {
	tests := []struct {
		name      string
		systemKey string
		promptKey string
	}{
		{name: "lesson extractor", systemKey: "lesson-extractor", promptKey: "lesson"},
		{name: "insight", systemKey: "insight", promptKey: "insight-analyzer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeSystemAgentDefaultForTest(t, tt.systemKey)
			rt := newSystemAgentResolveRuntime(t)

			_, got, err := rt.resolveAnalysisSystemAgent(tt.systemKey, db.Agent{Model: "caller-model"})
			if err != nil {
				t.Fatalf("resolve analysis system agent: %v", err)
			}
			want := rt.readPrompt(tt.promptKey)
			if got == "" {
				t.Fatal("resolve analysis system agent returned empty fallback prompt")
			}
			if got != want {
				t.Fatalf("fallback prompt = %q, want %q", got, want)
			}
		})
	}
}

func TestDisabledLessonExtractorRunsProductionFallback(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Disabled Lesson Extractor", System: true, SystemKey: "lesson-extractor", Soul: "disabled prompt", Model: "disabled-model", Disabled: true}); err != nil {
		t.Fatalf("create disabled lesson extractor: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Failed turn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rt.reflectLessons(ctx, session.ID, []lessonEvidence{{tool: "Bash", input: "false", errs: "exit status 1"}}, "")
	req := provider.captured()
	if req.System == "" || req.Model == "" {
		t.Fatalf("fallback prompt/model must be non-empty: %q/%q", req.System, req.Model)
	}
	if req.System != embeddedLessonPrompt || req.Model != "caller-model" {
		t.Fatalf("fallback prompt/model = %q/%q", req.System, req.Model)
	}
}

func TestDisabledInsightRunsProductionFallback(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Disabled Insight", System: true, SystemKey: "insight", Soul: "disabled prompt", Model: "disabled-model", Disabled: true}); err != nil {
		t.Fatalf("create disabled insight: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Failed turn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "assistant", Text: "failed", Steps: `[{"kind":"error","reason":"provider_error","text":"boom"}]`}); err != nil {
		t.Fatalf("add evidence: %v", err)
	}

	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}}, agent.ID); err != nil {
		t.Fatalf("RunInsightScan: %v", err)
	}
	req := provider.captured()
	if req.System == "" || req.Model == "" {
		t.Fatalf("fallback prompt/model must be non-empty: %q/%q", req.System, req.Model)
	}
	if req.System != embeddedInsightPrompt || req.Model != "caller-model" {
		t.Fatalf("fallback prompt/model = %q/%q", req.System, req.Model)
	}
}

func TestLessonExtractorResolveErrorRunsProductionFallback(t *testing.T) {
	removeSystemAgentDefaultForTest(t, "lesson-extractor")
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Failed turn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rt.reflectLessons(ctx, session.ID, []lessonEvidence{{tool: "Bash", input: "false", errs: "exit status 1"}}, "")
	req := provider.captured()
	if req.System != "You review a failed AI-agent turn and distill ONE reusable lesson for future turns.\nReply with 1-3 plain sentences: what failed, the likely root cause, and how to avoid it next time.\nGeneralize (a rule the agent can apply again), do not just restate the error.\nIf the failure is not generalizable (one-off cancellation, external outage, missing login), reply with exactly: NONE" {
		t.Fatalf("resolve-error lesson fallback prompt = %q", req.System)
	}
	if req.Model != "caller-model" {
		t.Fatalf("resolve-error lesson fallback model = %q, want caller-model", req.Model)
	}
}

func TestInsightResolveErrorRunsProductionFallback(t *testing.T) {
	removeSystemAgentDefaultForTest(t, "insight")
	rt := newSystemAgentResolveRuntime(t)
	provider := configureAnalysisTestProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "analysis-test", ProviderInstanceID: "analysis-test-instance", Model: "caller-model"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Failed turn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "assistant", Text: "failed", Steps: `[{"kind":"error","reason":"provider_error","text":"boom"}]`}); err != nil {
		t.Fatalf("add evidence: %v", err)
	}

	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}}, agent.ID); err != nil {
		t.Fatalf("RunInsightScan: %v", err)
	}
	req := provider.captured()
	if req.System != "You are a retrospective analyst for the TionHarness multi-agent runtime.\nYou are given ONE lens instruction and ONE past session's evidence (error steps + debug events).\nApply the lens strictly and emit high-signal findings only — no speculation, no restating raw errors.\nPrefer recurring, generalizable problems. If nothing qualifies, return an empty findings array." {
		t.Fatalf("resolve-error insight fallback prompt = %q", req.System)
	}
	if req.Model != "caller-model" {
		t.Fatalf("resolve-error insight fallback model = %q, want caller-model", req.Model)
	}
}

func removeSystemAgentDefaultForTest(t *testing.T, key string) {
	t.Helper()
	original := append([]SystemAgentDefinition(nil), systemAgentDefaults...)
	t.Cleanup(func() { systemAgentDefaults = original })
	filtered := make([]SystemAgentDefinition, 0, len(systemAgentDefaults)-1)
	for _, def := range systemAgentDefaults {
		if def.SystemKey != key {
			filtered = append(filtered, def)
		}
	}
	systemAgentDefaults = filtered
}
