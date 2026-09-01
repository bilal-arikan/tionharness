package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// sessionCtxProvider records the session id stamped on the context of every
// provider call, so a test can assert what an agent node's turn actually saw.
type sessionCtxProvider struct {
	mu   sync.Mutex
	seen []string
}

func (p *sessionCtxProvider) Name() string { return "flow-session-ctx-test" }

func (p *sessionCtxProvider) Complete(ctx context.Context, _ providers.Request) (*providers.Response, error) {
	p.mu.Lock()
	p.seen = append(p.seen, SessionIDFrom(ctx))
	p.mu.Unlock()
	return &providers.Response{Text: "node done"}, nil
}

func (p *sessionCtxProvider) sessionIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.seen...)
}

var (
	registerSessionCtxProvider sync.Once
	sessionCtxRecorder         = &sessionCtxProvider{}
)

func configureSessionCtxProvider(rt *Runtime) *sessionCtxProvider {
	registerSessionCtxProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "flow-session-ctx-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return sessionCtxRecorder, nil },
		))
	})
	rt.providers.SetInstances([]providers.Instance{{ID: "flow-session-ctx-test", KindID: "flow-session-ctx-test"}})
	sessionCtxRecorder.mu.Lock()
	sessionCtxRecorder.seen = nil
	sessionCtxRecorder.mu.Unlock()
	return sessionCtxRecorder
}

// TestFlowNodeTurnCarriesRunSessionID: a recorded flow run creates its own
// transcript session up front, and every node's turn must run WITH that session
// id on its context. Without the stamp the nodes run session-less, so
// session-scoped wiring (the autonomous Interaction server above all) resolves an
// empty session id and silently degrades.
func TestFlowNodeTurnCarriesRunSessionID(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	recorder := configureSessionCtxProvider(rt)
	ctx := context.Background()

	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "flow node", Provider: "flow-session-ctx-test", Model: "test"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}"},
		},
	}
	flowID := createFlow(t, rt, g)

	run, sessionID, err := rt.RunFlowRecorded(ctx, flowID, "go", false, nil)
	if err != nil {
		t.Fatalf("run flow recorded: %v", err)
	}
	if run.Status != db.FlowSuccess {
		t.Fatalf("flow run status = %q (%s), want success", run.Status, run.Error)
	}
	if sessionID == "" {
		t.Fatal("a recorded run must have a transcript session")
	}

	seen := recorder.sessionIDs()
	if len(seen) == 0 {
		t.Fatal("the agent node never reached the provider")
	}
	for i, got := range seen {
		if got != sessionID {
			t.Fatalf("node call %d ran with session id %q, want the run's session %q", i, got, sessionID)
		}
	}
	drainSpawns(t, rt)
}
