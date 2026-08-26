package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type systemSessionTestProvider struct {
	mu    sync.Mutex
	calls int
}

func (*systemSessionTestProvider) Name() string { return "system-session-test" }

func (p *systemSessionTestProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if strings.Contains(req.System, "retrospective analyst") {
		return &providers.Response{Text: `{"findings":[]}`}, nil
	}
	return &providers.Response{Text: "Inspect command prerequisites before retrying failures."}, nil
}

func (p *systemSessionTestProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

var (
	registerSystemSessionTestProvider sync.Once
	systemSessionProviderMu           sync.Mutex
	activeSystemSessionProvider       *systemSessionTestProvider
)

func configureSystemSessionTestProvider(rt *Runtime) *systemSessionTestProvider {
	registerSystemSessionTestProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "system-session-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) {
				systemSessionProviderMu.Lock()
				defer systemSessionProviderMu.Unlock()
				return activeSystemSessionProvider, nil
			},
		))
	})
	provider := &systemSessionTestProvider{}
	systemSessionProviderMu.Lock()
	activeSystemSessionProvider = provider
	systemSessionProviderMu.Unlock()
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{ID: "system-session-test-instance", KindID: "system-session-test"}})
	return provider
}

func createSystemSessionTestFixture(t *testing.T, system bool) (*Runtime, *systemSessionTestProvider, db.Session) {
	t.Helper()
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetLessonReflect(true)
	provider := configureSystemSessionTestProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "session owner", System: system, Provider: "system-session-test",
		ProviderInstanceID: "system-session-test-instance", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "failed turn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return rt, provider, session
}

func TestSystemAgentSessionDoesNotProduceLesson(t *testing.T) {
	rt, provider, session := createSystemSessionTestFixture(t, true)
	rt.reflectLessons(context.Background(), session.ID, []lessonEvidence{{tool: "Bash", errs: "exit status 1"}}, "")
	lessons, err := rt.db.ListLessons(0)
	if err != nil {
		t.Fatalf("list lessons: %v", err)
	}
	if len(lessons) != 0 || provider.callCount() != 0 {
		t.Fatalf("system session produced lesson/provider call: lessons=%d calls=%d", len(lessons), provider.callCount())
	}
}

func TestNormalAgentSessionProducesLesson(t *testing.T) {
	rt, provider, session := createSystemSessionTestFixture(t, false)
	rt.reflectLessons(context.Background(), session.ID, []lessonEvidence{{tool: "Bash", errs: "exit status 1"}}, "")
	lessons, err := rt.db.ListLessons(0)
	if err != nil {
		t.Fatalf("list lessons: %v", err)
	}
	if len(lessons) != 1 || provider.callCount() != 1 {
		t.Fatalf("normal session lesson/provider calls: lessons=%d calls=%d", len(lessons), provider.callCount())
	}
}

func TestInsightScanSkipsSystemAgentSession(t *testing.T) {
	rt, provider, session := createSystemSessionTestFixture(t, true)
	addInsightEvidence(t, rt, session.ID)
	result, err := rt.RunInsightScan(context.Background(), insight.ScanScope{LensIDs: []string{"tool-errors"}}, session.AgentID)
	if err != nil {
		t.Fatalf("run insight scan: %v", err)
	}
	if result.Sessions != 0 || result.Analyzed != 0 || provider.callCount() != 0 {
		t.Fatalf("system session scanned: sessions=%d analyzed=%d calls=%d", result.Sessions, result.Analyzed, provider.callCount())
	}
}

func TestInsightScanIncludesNormalAgentSession(t *testing.T) {
	rt, provider, session := createSystemSessionTestFixture(t, false)
	addInsightEvidence(t, rt, session.ID)
	result, err := rt.RunInsightScan(context.Background(), insight.ScanScope{LensIDs: []string{"tool-errors"}}, session.AgentID)
	if err != nil {
		t.Fatalf("run insight scan: %v", err)
	}
	if result.Sessions != 1 || result.Analyzed != 1 || provider.callCount() != 1 {
		t.Fatalf("normal session scan: sessions=%d analyzed=%d calls=%d", result.Sessions, result.Analyzed, provider.callCount())
	}
}

func addInsightEvidence(t *testing.T, rt *Runtime, sessionID string) {
	t.Helper()
	_, err := rt.db.AddMessage(context.Background(), db.Message{
		SessionID: sessionID, Role: "assistant", Text: "failed",
		Steps: `[{"kind":"error","reason":"provider_error","text":"boom"}]`,
	})
	if err != nil {
		t.Fatalf("add insight evidence: %v", err)
	}
}
