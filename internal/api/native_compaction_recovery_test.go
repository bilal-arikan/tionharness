package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

type nativeRecoveryProvider struct {
	calls            atomic.Int32
	afterCLI         func()
	fail             error
	failAfterSuccess bool
}

func (*nativeRecoveryProvider) Name() string { return "claude-cli" }
func (*nativeRecoveryProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return nil, errors.New("unexpected Complete call")
}
func (*nativeRecoveryProvider) NativeCompactionEvents() bool { return true }
func (p *nativeRecoveryProvider) CompactNative(_ context.Context, resumeID string, req providers.Request) (*providers.Response, error) {
	p.calls.Add(1)
	attemptID := "native-recovery-attempt"
	if req.OnCLICompaction != nil {
		req.OnCLICompaction(providers.CLICompactionEvent{
			Phase: providers.CLICompactionAttempt, Provider: p.Name(), AttemptID: attemptID, Attempt: 1,
			CLISessionIDIn: resumeID,
		})
	}
	if p.fail != nil && !p.failAfterSuccess {
		return nil, p.fail
	}
	if req.OnCLICompaction != nil {
		req.OnCLICompaction(providers.CLICompactionEvent{
			Phase: providers.CLICompactionSuccess, Provider: p.Name(), AttemptID: attemptID, Attempt: 1,
			CLISessionIDIn: resumeID, CLISessionIDOut: "new-resume",
		})
	}
	if p.afterCLI != nil {
		p.afterCLI()
	}
	if p.fail != nil {
		return nil, p.fail
	}
	return &providers.Response{
		SessionID: "new-resume",
		Trace:     []providers.TraceStep{{Kind: "compaction", Source: "cli-native", Provider: p.Name()}},
	}, nil
}

var nativeRecoveryKindID atomic.Uint64

func nativeRecoveryHarness(t *testing.T, provider *nativeRecoveryProvider) (*Server, *workspace.Workspace, db.Agent, db.Session, []db.Message) {
	t.Helper()
	s, wsp := newWorkspaceServer(t)
	resumeEnabled := true
	persistentDisabled := false
	if _, err := s.settings.Apply(settings.Patch{
		ClaudeResume: &resumeEnabled, ClaudePersistentSession: &persistentDisabled,
	}); err != nil {
		t.Fatal(err)
	}
	kindID := fmt.Sprintf("native-recovery-%d", nativeRecoveryKindID.Add(1))
	providers.RegisterKind(providers.NewBuiltinKind(
		providers.Manifest{Kind: kindID, Label: "Native recovery test", Transport: providers.TransportCLI},
		func(providers.ResolvedConfig) bool { return true },
		func(providers.ResolvedConfig) (providers.Provider, error) { return provider, nil },
	))
	s.providers.SetInstances([]providers.Instance{{ID: kindID, KindID: kindID, Enabled: true}})
	ctx := context.Background()
	agent, err := wsp.DB.CreateAgent(ctx, db.Agent{
		Name: "A", Provider: "claude-cli", ProviderInstanceID: kindID,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	history := []db.Message{{SessionID: session.ID, Role: providers.RoleUser, Text: "existing"}}
	if _, err := wsp.DB.AddMessage(ctx, history[0]); err != nil {
		t.Fatal(err)
	}
	if err := wsp.DB.SetSessionCLIResume(ctx, session.ID, "old-resume", 1); err != nil {
		t.Fatal(err)
	}
	session, err = wsp.DB.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, wsp, agent, session, history
}

func TestNativeCompactionPreflightMarkerFailureSkipsProvider(t *testing.T) {
	provider := &nativeRecoveryProvider{}
	s, wsp, _, session, history := nativeRecoveryHarness(t, provider)
	database := wsp.DB
	headerTmp := filepath.Join(database.Root(), "sessions", session.ID, "session.json.tmp")
	if err := os.Mkdir(headerTmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.runNativeCompact(context.Background(), wsp, session, history, nativeCompactAuto); err == nil {
		t.Fatal("native compaction unexpectedly passed a failed recovery-marker write")
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls.Load())
	}
	got, err := database.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLINativeCompactionPending {
		t.Fatal("failed preflight marker write advanced in-memory state")
	}
}

func TestNativeCompactionFinalWriteFailureForcesColdResume(t *testing.T) {
	provider := &nativeRecoveryProvider{}
	s, wsp, agent, session, history := nativeRecoveryHarness(t, provider)
	database := wsp.DB
	headerPath := filepath.Join(database.Root(), "sessions", session.ID, "session.json")
	provider.afterCLI = func() {
		if err := os.Mkdir(headerPath+".tmp", 0o755); err != nil {
			t.Fatalf("block final atomic write: %v", err)
		}
	}
	if _, err := s.runNativeCompact(context.Background(), wsp, session, history, nativeCompactAuto); err == nil {
		t.Fatal("native compaction unexpectedly reported success after final state write failed")
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls.Load())
	}
	got, err := database.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CLINativeCompactionPending || got.CLISessionID != "old-resume" || got.CLISentMsgCount != 1 || got.CLICompactMsgCount != 0 {
		t.Fatalf("failed final commit did not preserve fail-closed old state: %+v", got)
	}
	raw, err := os.ReadFile(headerPath)
	if err != nil {
		t.Fatal(err)
	}
	var durable db.Session
	if err := json.Unmarshal(raw, &durable); err != nil {
		t.Fatal(err)
	}
	if !durable.CLINativeCompactionPending || durable.CLISessionID != "old-resume" || durable.CLISentMsgCount != 1 {
		t.Fatalf("durable recovery state = %+v", durable)
	}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
	plan := s.planCLIResume(context.Background(), provider, 1, got, agent, history, false, &req)
	if !plan.active || !plan.coldStart || req.ResumeSessionID != "" {
		t.Fatalf("pending recovery resume plan = %+v request resume=%q", plan, req.ResumeSessionID)
	}
	if len(req.Messages) != 1 || req.Messages[0].Text != "full transcript" {
		t.Fatalf("pending recovery reused old delta: %+v", req.Messages)
	}
}

func TestNativeCompactionProviderFailureKeepsRecoveryMarker(t *testing.T) {
	provider := &nativeRecoveryProvider{fail: errors.New("provider failed")}
	s, wsp, _, session, history := nativeRecoveryHarness(t, provider)
	database := wsp.DB
	if _, err := s.runNativeCompact(context.Background(), wsp, session, history, nativeCompactAuto); err == nil {
		t.Fatal("provider failure unexpectedly succeeded")
	}
	got, err := database.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CLINativeCompactionPending {
		t.Fatal("provider failure cleared fail-closed recovery marker")
	}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
	plan := s.planCLIResume(context.Background(), provider, 1, got, db.Agent{ID: got.AgentID}, history, false, &req)
	if !plan.nativeCompactionRecovery || !plan.coldStart || req.ResumeSessionID != "" {
		t.Fatalf("provider failure recovery plan = %+v resume=%q", plan, req.ResumeSessionID)
	}
}

func TestNativeCompactionSuccessThenProviderErrorKeepsRecoveryMarker(t *testing.T) {
	provider := &nativeRecoveryProvider{fail: errors.New("provider failed after success"), failAfterSuccess: true}
	s, wsp, agent, session, history := nativeRecoveryHarness(t, provider)
	if _, err := s.runNativeCompact(context.Background(), wsp, session, history, nativeCompactAuto); err == nil {
		t.Fatal("provider error after success unexpectedly succeeded")
	}
	got, err := wsp.DB.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CLINativeCompactionPending {
		t.Fatal("provider success callback followed by error cleared recovery marker")
	}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
	plan := s.planCLIResume(context.Background(), provider, 1, got, agent, history, false, &req)
	if !plan.nativeCompactionRecovery || !plan.coldStart || req.ResumeSessionID != "" {
		t.Fatalf("mutation recovery plan = %+v resume=%q", plan, req.ResumeSessionID)
	}
}

func TestNativeCompactionRecoveryClearsWithNewResumeCommit(t *testing.T) {
	provider := &nativeRecoveryProvider{}
	s, wsp, agent, session, history := nativeRecoveryHarness(t, provider)
	ctx := context.Background()
	if err := wsp.DB.BeginSessionCLINativeCompaction(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := wsp.DB.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
	plan := s.planCLIResume(ctx, provider, 1, pending, agent, history, false, &req)
	if !plan.nativeCompactionRecovery || !plan.coldStart || req.ResumeSessionID != "" {
		t.Fatalf("pending recovery plan = %+v resume=%q", plan, req.ResumeSessionID)
	}
	_, err = wsp.DB.AddMessageWithCLIState(ctx, db.Message{
		SessionID: session.ID, AgentID: agent.ID, Role: providers.RoleAssistant, Text: "recovered",
	}, db.CLIReplyState{
		UpdateResume: true, ResumeSessionID: "recovered-resume", ResumeSentMsgCount: plan.sentCount + 1,
		ClearNativeCompactionPending: plan.nativeCompactionRecovery,
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := wsp.DB.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.CLINativeCompactionPending || recovered.CLISessionID != "recovered-resume" || recovered.CLISentMsgCount != 2 {
		t.Fatalf("recovered state = %+v", recovered)
	}
	nextHistory := append(append([]db.Message{}, history...),
		db.Message{Role: providers.RoleAssistant, Text: "recovered"},
		db.Message{Role: providers.RoleUser, Text: "next"},
	)
	nextReq := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full should be replaced"}}}
	next := s.planCLIResume(ctx, provider, 1, recovered, agent, nextHistory, false, &nextReq)
	if next.nativeCompactionRecovery || next.coldStart || nextReq.ResumeSessionID != "recovered-resume" {
		t.Fatalf("next resume plan = %+v resume=%q", next, nextReq.ResumeSessionID)
	}
	if err := wsp.DB.BeginSessionCLINativeCompaction(ctx, session.ID); err != nil {
		t.Fatalf("new native compaction remained blocked: %v", err)
	}
}

func TestNativeCompactionRecoveryEmptyResumeRetiresAuthorityWithReply(t *testing.T) {
	provider := &nativeRecoveryProvider{}
	s, wsp, agent, session, history := nativeRecoveryHarness(t, provider)
	ctx := context.Background()
	if err := wsp.DB.BeginSessionCLINativeCompaction(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := wsp.DB.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
	plan := s.planCLIResume(ctx, provider, 1, pending, agent, history, false, &req)
	if !plan.nativeCompactionRecovery || !plan.active || !plan.coldStart {
		t.Fatalf("recovery plan = %+v", plan)
	}
	if _, err := wsp.DB.AddMessageWithCLIState(ctx, db.Message{
		SessionID: session.ID, AgentID: agent.ID, Role: providers.RoleAssistant, Text: "cold response without id",
	}, db.CLIReplyState{
		RetireResume: true, ClearNativeCompactionPending: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := wsp.DB.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLINativeCompactionPending || got.CLISessionID != "" || got.CLISentMsgCount != 0 || got.CLICompactMsgCount != 0 {
		t.Fatalf("empty-id recovery state = %+v", got)
	}
	if err := wsp.DB.BeginSessionCLINativeCompaction(ctx, session.ID); err != nil {
		t.Fatalf("retired recovery still blocks native compaction: %v", err)
	}
}

func TestNativeCompactionRecoveryIncompatibleResumeRetiresBeforeTurn(t *testing.T) {
	cases := []struct {
		name         string
		resume       bool
		agentCount   int
		participants []string
	}{
		{name: "resume disabled", resume: false, agentCount: 1},
		{name: "multi participant", resume: true, agentCount: 1, participants: []string{"AGT1", "AGT2"}},
		{name: "multi agent", resume: true, agentCount: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &nativeRecoveryProvider{}
			s, wsp, agent, session, history := nativeRecoveryHarness(t, provider)
			if _, err := s.settings.Apply(settings.Patch{ClaudeResume: &tc.resume}); err != nil {
				t.Fatal(err)
			}
			if err := wsp.DB.BeginSessionCLINativeCompaction(context.Background(), session.ID); err != nil {
				t.Fatal(err)
			}
			pending, err := wsp.DB.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			pending.Participants = tc.participants
			req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "full transcript"}}}
			plan := s.planCLIResume(context.Background(), provider, tc.agentCount, pending, agent, history, false, &req)
			if !plan.nativeCompactionRecovery || !plan.retireNativeRecovery || plan.active || req.ResumeSessionID != "" {
				t.Fatalf("incompatible recovery plan = %+v resume=%q", plan, req.ResumeSessionID)
			}
			if err := wsp.DB.RetireSessionCLINativeCompactionRecovery(context.Background(), session.ID); err != nil {
				t.Fatal(err)
			}
			got, err := wsp.DB.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.CLINativeCompactionPending || got.CLISessionID != "" || got.CLISentMsgCount != 0 || got.CLICompactMsgCount != 0 {
				t.Fatalf("retired state = %+v", got)
			}
		})
	}
}
