package api

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

type blockingPanicRecoveryHandler struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*blockingPanicRecoveryHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *blockingPanicRecoveryHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "queued turn panicked" {
		h.once.Do(func() { close(h.entered) })
		<-h.release
	}
	return nil
}
func (h *blockingPanicRecoveryHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *blockingPanicRecoveryHandler) WithGroup(string) slog.Handler      { return h }

type blockingGenerationTitleProvider struct {
	entered chan<- struct{}
	release <-chan struct{}
}

func (*blockingGenerationTitleProvider) Name() string { return "generation-title-test" }

func (p *blockingGenerationTitleProvider) Complete(ctx context.Context, _ providers.Request) (*providers.Response, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
		return &providers.Response{Text: `{"title":"Stale Generated Title"}`}, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

var (
	registerGenerationTitleProvider sync.Once
	generationTitleProviderMu       sync.Mutex
	generationTitleProvider         *blockingGenerationTitleProvider
)

func configureGenerationTitleProvider(s *Server, provider *blockingGenerationTitleProvider) {
	registerGenerationTitleProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "generation-title-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) {
				generationTitleProviderMu.Lock()
				defer generationTitleProviderMu.Unlock()
				return generationTitleProvider, nil
			},
		))
	})
	generationTitleProviderMu.Lock()
	generationTitleProvider = provider
	generationTitleProviderMu.Unlock()
	s.providers.SetInstances([]providers.Instance{{ID: "generation-title-instance", KindID: "generation-title-test", Enabled: true}})
}

func TestSessionGenerationGateSerializesActivationAndFencedWrite(t *testing.T) {
	runs := newChatRuns()
	runA := runs.register("run-A", "session", "workspace", func() {})
	runB := runs.register("run-B", "session", "workspace", func() {})
	runs.activate(runA)

	releaseA, current := runs.acquireCurrent(runA)
	if !current {
		t.Fatal("run A did not acquire its generation fence")
	}
	activatedB := make(chan struct{})
	go func() {
		runs.activate(runB)
		close(activatedB)
	}()
	select {
	case <-activatedB:
		t.Fatal("run B activated while run A fenced callback was active")
	case <-time.After(30 * time.Millisecond):
	}
	releaseA()
	select {
	case <-activatedB:
	case <-time.After(time.Second):
		t.Fatal("run B did not activate after run A callback released")
	}
	if release, ok := runs.acquireCurrent(runA); ok {
		release()
		t.Fatal("run A acquired a new write after run B activation")
	}
}

func TestGenerationGateDoesNotSerializeDifferentSessions(t *testing.T) {
	runs := newChatRuns()
	runA := runs.register("run-A", "session-A", "workspace", func() {})
	runB := runs.register("run-B", "session-B", "workspace", func() {})
	runs.activate(runA)
	releaseA, current := runs.acquireCurrent(runA)
	if !current {
		t.Fatal("run A did not acquire its generation fence")
	}
	defer releaseA()

	done := make(chan struct{})
	go func() {
		runs.activate(runB)
		releaseB, ok := runs.acquireCurrent(runB)
		if ok {
			releaseB()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session B activation/write blocked behind session A callback")
	}
}

func TestStalePanicRecoveryWritesNothingAfterNewGenerationActivates(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "panic fence"})
	if err != nil {
		t.Fatal(err)
	}

	runA := s.runs.register("run-A", session.ID, wsp.ID, func() {})
	s.runs.activate(runA)
	releaseRecoveryRef := s.runs.retain(runA)
	defer releaseRecoveryRef()

	handler := &blockingPanicRecoveryHandler{entered: make(chan struct{}), release: make(chan struct{})}
	s.logger = slog.New(handler)
	recovered := make(chan struct{})
	go func() {
		defer close(recovered)
		defer func() {
			if recoveredValue := recover(); recoveredValue != nil {
				s.recoverQueuedTurnPanic(wsp, chatReq{SessionID: session.ID, ClientMsgID: "old-client"}, runA, recoveredValue)
			}
		}()
		defer s.runs.unregister(runA.id)
		panic("old generation panic")
	}()

	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("panic recovery did not reach controlled write boundary")
	}

	// unregister ran before recovery. The retained gate lets B reuse the same
	// generation sequence and supersede A while recovery is paused before fencing.
	runB := s.runs.register("run-B", session.ID, wsp.ID, func() {})
	s.runs.activate(runB)
	if runB.gate != runA.gate || runB.generation <= runA.generation {
		t.Fatalf("new run did not supersede old generation on retained gate: A=%d B=%d sameGate=%v",
			runA.generation, runB.generation, runB.gate == runA.gate)
	}
	s.publishHub(wsp.ID, session.ID, sessionhub.KindStep, map[string]string{"owner": "B"}, false)
	close(handler.release)
	select {
	case <-recovered:
	case <-time.After(time.Second):
		t.Fatal("panic recovery did not return")
	}

	messages, err := wsp.DB.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("stale recovery persisted durable error/reply: %+v", messages)
	}
	debugEvents, err := wsp.DB.ReadDebugEvents(ctx, session.ID, db.DebugError, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(debugEvents) != 0 {
		t.Fatalf("stale recovery persisted debug events: %+v", debugEvents)
	}
	events, ok := s.hub.Replay(wsp.ID, session.ID, 0)
	if !ok || len(events) != 1 || events[0].Kind != sessionhub.KindStep {
		t.Fatalf("stale recovery published turn_error or committed B event: ok=%v events=%+v", ok, events)
	}
}

func TestSessionGenerationGateReleasedAfterLastRun(t *testing.T) {
	runs := newChatRuns()
	runA := runs.register("run-A", "session", "workspace", func() {})
	runs.register("run-B", "session", "workspace", func() {})
	if len(runs.gates) != 1 || runA.gate.refs != 2 {
		t.Fatalf("gate state after register: gates=%d refs=%d", len(runs.gates), runA.gate.refs)
	}
	runs.unregister("run-A")
	if len(runs.gates) != 1 || runA.gate.refs != 1 {
		t.Fatalf("gate removed while another run referenced it: gates=%d refs=%d", len(runs.gates), runA.gate.refs)
	}
	runs.unregister("run-B")
	if len(runs.gates) != 0 {
		t.Fatalf("gate leaked after final unregister: %d", len(runs.gates))
	}
}

func TestDetachedRunCannotOverwriteNewGeneration(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "generation", Provider: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}

	runA := s.runs.register("run-A", session.ID, wsp.ID, func() {})
	s.runs.activate(runA)
	sseEvents := 0
	turnA := &chatTurn{s: s, wsp: wsp, database: wsp.DB, req: chatReq{SessionID: session.ID}, run: runA, runID: "run-A", ctx: ctx, session: session, agents: []db.Agent{agentRow}, sse: func(string, any) { sseEvents++ }, inflightID: "reply-A"}
	if err := wsp.DB.WriteInflight(db.InflightTurn{MessageID: "reply-A", RunID: "run-A", Generation: runA.generation, SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}

	// Detach permits B to register while A's provider goroutine is still alive.
	runB := s.runs.register("run-B", session.ID, wsp.ID, func() {})
	s.runs.activate(runB)
	turnB := &chatTurn{s: s, wsp: wsp, database: wsp.DB, req: chatReq{SessionID: session.ID}, run: runB, runID: "run-B", ctx: ctx, session: session, sse: func(string, any) {}, inflightID: "reply-B"}
	if err := wsp.DB.WriteInflight(db.InflightTurn{MessageID: "reply-B", RunID: "run-B", Generation: runB.generation, SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}

	// A returns late. Its live step, terminal reply and sidecar clear are fenced.
	var partialA strings.Builder
	var keptA []agent.TurnStep
	turnA.streamSink(ctx, &partialA, &keptA, func() {})(agent.TurnStep{Kind: agent.StepDelta, Text: "late-A"})
	if partialA.Len() != 0 || len(keptA) != 0 {
		t.Fatal("superseded run applied a late live step")
	}
	if turnA.persistAgentReply(agentRow, agentTurnPrep{}, "reply-A", time.Now(), nil, nil, &providers.Response{Text: "late-A"}) {
		t.Fatal("superseded run persisted terminal reply")
	}
	turnA.emitLifecycleSteps([]agent.TurnStep{{Kind: agent.StepText, Text: "late lifecycle"}})
	if turnA.persistBlockedPrompt(nil, "late block") {
		t.Fatal("superseded run persisted block reply")
	}
	turnA.failTurn(agentRow.ID, "late_failure", "late failure")
	turnA.finishTurn()
	turnA.clearInflight()
	if inflight, ok, err := wsp.DB.ReadInflight(session.ID); err != nil || !ok || inflight.MessageID != "reply-B" {
		t.Fatalf("old run cleared new inflight state: got=%+v ok=%v err=%v", inflight, ok, err)
	}
	if sseEvents != 0 {
		t.Fatalf("superseded run emitted %d live events", sseEvents)
	}
	if events, _ := s.hub.Replay(wsp.ID, session.ID, 0); len(events) != 0 {
		t.Fatalf("superseded run published hub events: %+v", events)
	}

	if !turnB.persistAgentReply(agentRow, agentTurnPrep{}, "reply-B", time.Now(), nil, []agent.TurnStep{{Kind: agent.StepText, Text: "B-step"}}, &providers.Response{Text: "reply-B"}) {
		t.Fatal("current run failed to persist reply")
	}
	messages, err := wsp.DB.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Text != "reply-B" || strings.Contains(messages[0].Steps, "late-A") {
		t.Fatalf("generation fence failed: %+v", messages)
	}
	if _, ok, err := wsp.DB.ReadInflight(session.ID); err != nil || ok {
		t.Fatalf("current run sidecar not cleared after its own terminal persist: ok=%v err=%v", ok, err)
	}
}

func TestQueuedRegistrationDoesNotSupersedeSlotOwner(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "generation", Provider: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}

	runA := s.runs.register("run-A", session.ID, wsp.ID, func() {})
	s.runs.activate(runA)
	turnA := &chatTurn{s: s, wsp: wsp, database: wsp.DB, req: chatReq{SessionID: session.ID}, run: runA, runID: "run-A", ctx: ctx, session: session, agents: []db.Agent{agentRow}, sse: func(string, any) {}}
	runB := s.runs.register("run-B", session.ID, wsp.ID, func() {})
	if runB.generation != 0 {
		t.Fatalf("queued run generation = %d, want inactive", runB.generation)
	}
	if !turnA.persistAgentReply(agentRow, agentTurnPrep{}, "reply-A", time.Now(), nil, []agent.TurnStep{{Kind: agent.StepText, Text: "A-step"}}, &providers.Response{Text: "reply-A"}) {
		t.Fatal("slot owner was invalidated by queued registration")
	}
	turnA.finishTurn()

	s.runs.activate(runB)
	var partial strings.Builder
	var kept []agent.TurnStep
	turnA.streamSink(ctx, &partial, &kept, func() {})(agent.TurnStep{Kind: agent.StepDelta, Text: "stale-A"})
	if partial.Len() != 0 {
		t.Fatal("old owner remained writable after queued run activation")
	}
}

func TestAutoTitleProviderRunsOutsideGateAndStaleCommitIsDropped(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	entered := make(chan struct{}, 1)
	releaseProvider := make(chan struct{})
	configureGenerationTitleProvider(s, &blockingGenerationTitleProvider{entered: entered, release: releaseProvider})
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{
		Name:               "generation title",
		Provider:           "generation-title-test",
		ProviderInstanceID: "generation-title-instance",
		Model:              "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionA, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	runA := s.runs.register("run-A", sessionA.ID, wsp.ID, func() {})
	s.runs.activate(runA)
	sseEvents := 0
	turnA := &chatTurn{
		s: s, wsp: wsp, database: wsp.DB,
		req: chatReq{SessionID: sessionA.ID, Message: "title this"},
		run: runA, runID: "run-A", ctx: ctx, session: sessionA,
		agents: []db.Agent{agentRow}, firstTurn: true,
		sse: func(string, any) { sseEvents++ },
	}
	finished := make(chan struct{})
	go func() {
		turnA.finishTurn()
		close(finished)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("title provider was not called")
	}

	// A blocked title provider must not hold the global registry or any other
	// session's generation gate. Exercise register, generation activation,
	// session-info lookup and cancellation while the provider remains blocked.
	otherCancelled := make(chan struct{})
	var cancelOther sync.Once
	runOther := s.runs.register("run-other", sessionB.ID, wsp.ID, func() {
		cancelOther.Do(func() { close(otherCancelled) })
	})
	otherDone := make(chan struct{})
	go func() {
		s.runs.activate(runOther)
		release, ok := s.runs.acquireCurrent(runOther)
		if ok {
			release()
		}
		info, live := s.runs.sessionRunInfo(wsp.ID, sessionB.ID)
		if !live || info.RunID != runOther.id {
			t.Errorf("session info missed other run: live=%v info=%+v", live, info)
		}
		runOther.cancel()
		close(otherDone)
	}()
	select {
	case <-otherDone:
	case <-time.After(time.Second):
		t.Fatal("session B register/activation/info/cancel blocked behind session A title provider")
	}
	select {
	case <-otherCancelled:
	case <-time.After(time.Second):
		t.Fatal("session B cancellation did not progress while session A title provider was blocked")
	}

	// Supersede A after title generation started but before its candidate can be
	// committed. Activation must not wait for the provider either.
	runSuccessor := s.runs.register("run-successor", sessionA.ID, wsp.ID, func() {})
	activated := make(chan struct{})
	go func() {
		s.runs.activate(runSuccessor)
		close(activated)
	}()
	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatal("successor activation blocked behind title provider")
	}
	close(releaseProvider)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("stale title turn did not finish")
	}

	gotSession, err := wsp.DB.GetSession(ctx, sessionA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.Title != "" {
		t.Fatalf("stale title persisted: %q", gotSession.Title)
	}
	if sseEvents != 0 || turnA.emitted {
		t.Fatalf("stale terminal effects escaped fence: sse=%d emitted=%v", sseEvents, turnA.emitted)
	}
	if events, _ := s.hub.Replay(wsp.ID, sessionA.ID, 0); len(events) != 0 {
		t.Fatalf("stale terminal hub events persisted: %+v", events)
	}
}
