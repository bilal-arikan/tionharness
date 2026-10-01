package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func decisionWorkflowFixture(t *testing.T, modes map[string]decider.Mode) (*Runtime, *decisionStub, context.Context, db.Agent) {
	t.Helper()
	rt, tun := newTestRuntime(t, t.TempDir())
	t.Cleanup(func() { drainSpawns(t, rt) })
	stub := newDecisionStub(t)
	wireDecider(t, tun, stub, modes)
	agent, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Workflow", Provider: "anthropic", Model: "claude-sonnet-4-5", MCPEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := rt.db.CreateSession(context.Background(), db.Session{AgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	return rt, stub, WithSessionID(context.Background(), session.ID), agent
}

func TestDecisionWorkflowsOffMakesNoJudgeCalls(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, nil)
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "Research this task"}}}
	rt.prepareDecisionSetup(ctx, agent, &req)
	rt.routeDecisionTurn(ctx, agent, nil, req)
	rt.reviewDecisionFold(ctx, agent, "", []conversation.FoldSegment{{Key: "a", Text: "remember", Rendered: "remember"}})
	rt.applyDecisionContext(ctx, agent, &req)
	rt.ReviewClarification(ctx, agent, json.RawMessage(`{"question":"Which color?","required_input":false}`))
	if got := rt.reviewDecisionWorker(ctx, SessionIDFrom(ctx), "worker-1", "original"); got != "original" {
		t.Fatal("disabled worker review changed delivery")
	}
	if stub.calls() != 0 || req.System != "" || req.SystemDynamic != "" {
		t.Fatalf("disabled workflows changed behavior or called the judge: %d", stub.calls())
	}
}

func TestDecisionSetupBatchesLoadsAndActivatesPermittedCapabilities(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authSessionSetup: decider.ModeOn})
	for _, slug := range []string{"workflow-research", "workflow-private"} {
		if _, err := rt.Skills().Create(slug, skills.SkillInput{Name: slug, Description: "Research workflow evidence", Body: "Use explicit evidence: " + slug}); err != nil {
			t.Fatal(err)
		}
	}
	agent.Skills = []string{"workflow-research"}
	lazy := rt.LazyToolCatalog(ctx, agent)
	if len(lazy) == 0 {
		t.Fatal("fixture has no lazy tools")
	}
	tool := lazy[0]
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "workflow research " + tool.Name}}}
	items := []db.DecisionItem{{Key: "skill:workflow-research", Kind: "skill", Label: "workflow-research — Research workflow evidence "}}
	for _, def := range lazy {
		items = append(items, db.DecisionItem{Key: "tool:" + def.Name, Kind: "tool", Label: policyText(def.Name+" — "+def.Description, 400)})
	}
	items = rankSetupCandidates(items, latestPolicyPrompt(req), rt.policyConfig(authSessionSetup).CandidateLimit)
	stub.set(func(stub *decisionStub) {
		for i, item := range items {
			stub.noul[questionKey(i)] = 0.01
			if item.Key == "skill:workflow-research" || item.Key == "tool:"+tool.Name {
				stub.noul[questionKey(i)] = 0.99
			}
		}
	})
	rt.prepareDecisionSetup(ctx, agent, &req)
	if stub.calls() != 1 || !strings.Contains(req.System, "Use explicit evidence: workflow-research") || strings.Contains(req.System, "workflow-private") {
		t.Fatalf("setup did not load permitted skill in one batch: calls=%d system=%q", stub.calls(), req.System)
	}
	stub.mu.Lock()
	body := stub.bodies[0]
	stub.mu.Unlock()
	if !strings.Contains(body, "skill:workflow-research") || !strings.Contains(body, "tool:"+tool.Name) || strings.Contains(body, "workflow-private") {
		t.Fatalf("judge candidates were not permission-filtered: %s", body)
	}
	reg := rt.buildRegistry(ctx, agent)
	active := tools.NewActiveTools()
	rt.seedDecisionTools(ctx, agent, reg, active)
	found := false
	for _, def := range reg.ActiveDefs(rt.toolFilter(ctx, agent), active.Snapshot()) {
		found = found || def.Name == tool.Name
	}
	if !active.Has(tool.Name) || !found {
		t.Fatal("selected lazy tool did not ship a real schema")
	}
	// Revoking a tool between turns must override the saved selection.
	agent.BlockedTools = `["` + tool.Name + `"]`
	denied := tools.NewActiveTools()
	rt.seedDecisionTools(ctx, agent, reg, denied)
	if denied.Has(tool.Name) {
		t.Fatal("persisted tool choice bypassed a current permission restriction")
	}
	rt.prepareDecisionSetup(ctx, agent, &providers.Request{})
	if stub.calls() != 1 {
		t.Fatal("setup reran the batch on the same session")
	}
}

func TestDecisionShadowRecordsRecommendationsWithoutSideEffects(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authClarification: decider.ModeShadow, authWorkerReview: decider.ModeShadow, authCompactRetention: decider.ModeShadow})
	stub.set(func(stub *decisionStub) {
		stub.noul["answered"] = 0.99
		for i := 0; i < 5; i++ {
			stub.noul[questionKey(i)] = 0.99
			stub.choice[questionKey(i)] = "drop"
		}
	})
	if _, skip := rt.ReviewClarification(ctx, agent, json.RawMessage(`{"question":"Which color?","required_input":false}`)); skip {
		t.Fatal("shadow clarification suppressed user input")
	}
	if got := rt.reviewDecisionWorker(ctx, SessionIDFrom(ctx), "worker", "original"); got != "original" {
		t.Fatal("shadow review changed worker delivery")
	}
	plan := rt.reviewDecisionFold(ctx, agent, "", []conversation.FoldSegment{{Key: "old", SourceID: "message", Text: "old", Rendered: "old rendered"}})
	if plan.Rendered != "old rendered" || plan.Protected != "" {
		t.Fatal("shadow fold modified context")
	}
	waitBackground(rt)
	state, err := rt.db.ReadSessionDecisions(ctx, SessionIDFrom(ctx))
	if err != nil || len(state.Entries) != 3 || len(state.Memories) != 0 {
		t.Fatalf("shadow state: %+v, %v", state, err)
	}
	for _, entry := range state.Entries {
		if entry.Status != "observed" || entry.Applied != entry.Baseline {
			t.Fatalf("shadow event reported an application: %+v", entry)
		}
	}
}

func TestDecisionClarificationRespectsRequiredInputAndApproval(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authClarification: decider.ModeOn})
	stub.set(func(stub *decisionStub) { stub.noul["answered"] = 0.99 })
	for _, test := range []struct {
		input string
		skip  bool
	}{
		{`{"question":"Which color?","required_input":false}`, true},
		{`{"question":"Which color?","required_input":true}`, false},
		{`{"question":"Which color?"}`, false},
		{`{"question":"Approve deployment?","required_input":false}`, false},
		{`{"question":"May I delete this file?","required_input":false}`, false},
	} {
		if _, skip := rt.ReviewClarification(ctx, agent, json.RawMessage(test.input)); skip != test.skip {
			t.Errorf("clarification %s: skip=%v, want %v", test.input, skip, test.skip)
		}
	}
	stub.set(func(stub *decisionStub) { stub.noul["answered"] = 0.05 })
	if _, skip := rt.ReviewClarification(ctx, agent, json.RawMessage(`{"question":"Which color?","required_input":false}`)); skip {
		t.Fatal("unanswered optional question was suppressed")
	}
}

func TestDecisionCompactFiltersWithoutDeletingCanonicalHistory(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authCompactRetention: decider.ModeOn})
	segments := []conversation.FoldSegment{}
	for _, text := range []string{"obsolete", "constraint", "normal", "pinned"} {
		msg, err := rt.db.AddMessage(ctx, db.Message{SessionID: SessionIDFrom(ctx), Role: "user", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		segments = append(segments, conversation.FoldSegment{Key: "context:" + msg.ID, SourceID: msg.ID, Role: msg.Role, Text: msg.Text, Rendered: msg.Text + "\n"})
	}
	if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
		state.Memories = []db.DecisionMemory{{Key: segments[3].Key, SourceID: segments[3].SourceID, Pinned: true}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	segments[2].Mandatory = true
	stub.set(func(stub *decisionStub) {
		stub.choice[questionKey(0)] = "drop"
		stub.choice[questionKey(1)] = "keep"
		stub.choice[questionKey(2)] = "drop"
		stub.choice[questionKey(3)] = "drop"
	})
	plan := rt.reviewDecisionFold(ctx, agent, "", segments)
	if strings.Contains(plan.Rendered, "obsolete") || !strings.Contains(plan.Rendered, "normal") || !strings.Contains(plan.Protected, "constraint") || !strings.Contains(plan.Protected, "pinned") {
		t.Fatalf("incorrect triage: %+v", plan)
	}
	messages, err := rt.db.ListMessages(ctx, SessionIDFrom(ctx))
	if err != nil || len(messages) != 4 || messages[0].Text != "obsolete" {
		t.Fatalf("triage altered canonical transcript: %+v, %v", messages, err)
	}
	// Low confidence must preserve the ordinary summary input, while the
	// deterministic pin remains verbatim regardless of classifier confidence.
	hub := rt.deciderHub()
	cfg := hub.Config()
	ac := cfg.Authorities[authCompactRetention]
	ac.Threshold = 0.95
	cfg.Authorities[authCompactRetention] = ac
	if _, err := hub.Update(cfg); err != nil {
		t.Fatal(err)
	}
	plan = rt.reviewDecisionFold(ctx, agent, "", segments)
	if !strings.Contains(plan.Rendered, "obsolete") || !strings.Contains(plan.Protected, "pinned") {
		t.Fatalf("low-confidence decision discarded history or a manual pin: %+v", plan)
	}
}

func TestDecisionRemindersWaitForConfiguredCompactInterval(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authContextReminder: decider.ModeOn})
	stub.set(func(stub *decisionStub) { stub.noul[questionKey(0)] = 0.99 })
	if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
		state.CompactCount = 3
		state.Memories = []db.DecisionMemory{{Key: "remember", Label: "User constraint", Text: "Use Windows paths", AddedAtCompact: 1}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := providers.Request{}
	rt.applyDecisionContext(ctx, agent, &req)
	if req.SystemDynamic != "" || stub.calls() != 0 {
		t.Fatal("newly retained memory was reminded before three later compacts")
	}
	rt.recordDecisionCompact(ctx)
	req = providers.Request{}
	rt.applyDecisionContext(ctx, agent, &req)
	if !strings.Contains(req.SystemDynamic, "Use Windows paths") || stub.calls() != 1 {
		t.Fatalf("due reminder was not injected: %q calls=%d", req.SystemDynamic, stub.calls())
	}
	for count := 4; count <= 6; count++ {
		if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error { state.CompactCount = count; return nil }); err != nil {
			t.Fatal(err)
		}
		req = providers.Request{}
		rt.applyDecisionContext(ctx, agent, &req)
		if stub.calls() != 1 || req.SystemDynamic != "" {
			t.Fatal("reminder repeated before three additional compacts")
		}
	}
	rt.recordDecisionCompact(ctx)
	req = providers.Request{}
	rt.applyDecisionContext(ctx, agent, &req)
	if stub.calls() != 2 || !strings.Contains(req.SystemDynamic, "Use Windows paths") {
		t.Fatal("reminder did not repeat when the interval elapsed")
	}
}

func TestDecisionRemindersRecordOnlyFragmentsActuallyInjected(t *testing.T) {
	for _, missingSource := range []bool{true, false} {
		name := "missing canonical source"
		if !missingSource {
			name = "pinned context exhausts reminder budget"
		}
		t.Run(name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authContextReminder: decider.ModeOn})
			hub := rt.deciderHub()
			cfg := hub.Config()
			ac := cfg.Authorities[authContextReminder]
			ac.ContextBudget = 1024
			cfg.Authorities[authContextReminder] = ac
			if _, err := hub.Update(cfg); err != nil {
				t.Fatal(err)
			}
			unusable := db.DecisionMemory{Key: "unusable", Label: "Selected but unusable", AddedAtCompact: 1, LastReminded: 1}
			memories := []db.DecisionMemory{}
			if missingSource {
				unusable.SourceID = "missing-canonical-message"
				unusable.Text = "Never inject a fallback for a missing canonical source"
			} else {
				unusable.Text = strings.Repeat("X", 500)
				memories = append(memories, db.DecisionMemory{Key: "pinned", Text: strings.Repeat("P", 700), Pinned: true})
			}
			memories = append(memories, unusable)
			if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
				state.CompactCount = 4
				state.Memories = memories
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			stub.set(func(stub *decisionStub) {
				stub.noul[questionKey(0)] = 0.99
				stub.noul[questionKey(1)] = 0.99
			})
			req := providers.Request{}
			rt.applyDecisionContext(ctx, agent, &req)
			state, err := rt.db.ReadSessionDecisions(ctx, SessionIDFrom(ctx))
			if err != nil || len(state.Entries) != 1 || state.Entries[0].Recommended != "unusable" || state.Entries[0].Applied != "none" {
				t.Fatalf("unavailable reminder falsely reported application: %+v, %v", state.Entries, err)
			}
			for _, memory := range state.Memories {
				if memory.Key == "unusable" && memory.LastReminded != 1 {
					t.Fatal("reminder cooldown advanced without injecting its fragment")
				}
			}
			if strings.Contains(req.SystemDynamic, unusable.Text) || strings.Contains(req.SystemDynamic, "[unusable]") {
				t.Fatal("unavailable reminder reached the actual prompt")
			}
			// A small canonical fragment is genuinely available and fits after the
			// same pinned context. Only this fragment may advance its cooldown.
			message, err := rt.db.AddMessage(ctx, db.Message{SessionID: SessionIDFrom(ctx), Role: "user", Text: "Valid canonical reminder"})
			if err != nil {
				t.Fatal(err)
			}
			if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
				state.Memories = append(state.Memories, db.DecisionMemory{Key: "valid", Label: "Valid reminder", SourceID: message.ID, AddedAtCompact: 1, LastReminded: 1})
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			req = providers.Request{}
			rt.applyDecisionContext(ctx, agent, &req)
			state, err = rt.db.ReadSessionDecisions(ctx, SessionIDFrom(ctx))
			if err != nil || len(state.Entries) != 2 || state.Entries[1].Applied != "valid" || !strings.Contains(req.SystemDynamic, message.Text) {
				t.Fatalf("fitted canonical reminder was not applied accurately: entries=%+v prompt=%q err=%v", state.Entries, req.SystemDynamic, err)
			}
			for _, memory := range state.Memories {
				if memory.Key == "unusable" && memory.LastReminded != 1 || memory.Key == "valid" && memory.LastReminded != 4 {
					t.Fatalf("cooldown does not match actual prompt injection: %+v", memory)
				}
			}
		})
	}
}

func TestDecisionWorkerReviewAugmentsAndFailurePreservesDelivery(t *testing.T) {
	rt, stub, ctx, _ := decisionWorkflowFixture(t, map[string]decider.Mode{authWorkerReview: decider.ModeOn})
	stub.set(func(stub *decisionStub) { stub.noul[questionKey(0)] = 0.99 })
	got := rt.reviewDecisionWorker(ctx, SessionIDFrom(ctx), "worker", "The implementation passed tests.")
	if !strings.HasPrefix(got, "The implementation passed tests.") || !strings.Contains(got, "<worker_review>") || !strings.Contains(got, "Verify claimed results") {
		t.Fatalf("review did not augment the delivery: %q", got)
	}
	stub.set(func(stub *decisionStub) { stub.status = 400 })
	if got := rt.reviewDecisionWorker(ctx, SessionIDFrom(ctx), "worker", "Original note"); got != "Original note" {
		t.Fatalf("judge failure changed delivery: %q", got)
	}
}

func TestDecisionRouterRestrictsCandidatesAndKeepsPinnedOrResumedRoutes(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		threshold float64
		resume    bool
		pin       bool
		wantModel string
		wantCalls int
	}{
		{"select available native", 0.8, false, false, "claude-opus-4-1", 1},
		{"low confidence keeps baseline", 0.95, false, false, "claude-sonnet-4-5", 1},
		{"resume keeps baseline", 0.8, true, false, "claude-sonnet-4-5", 0},
		{"manual pin overrides judge", 0.8, false, true, "claude-opus-4-1", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authModelRouter: decider.ModeOn})
			rt.providers.SetInstances([]providers.Instance{
				{ID: "a-native", KindID: "anthropic", Enabled: true, Models: "claude-sonnet-4-5", Values: map[string]string{providers.FieldKeyAPIKey: "fixture"}},
				{ID: "b-native", KindID: "anthropic", Enabled: true, Models: "claude-opus-4-1", Values: map[string]string{providers.FieldKeyAPIKey: "fixture"}},
				{ID: "disabled", KindID: "anthropic", Enabled: false, Models: "claude-opus-disabled", Values: map[string]string{providers.FieldKeyAPIKey: "fixture"}},
				{ID: "missing-key", KindID: "anthropic", Enabled: true, Models: "claude-opus-unavailable"},
				{ID: "cli", KindID: "claude-cli", Enabled: true, Models: "opus", Values: map[string]string{providers.FieldKeyCLIPath: executable}},
			})
			agent.ProviderInstanceID, agent.Model = "a-native", "claude-sonnet-4-5"
			provider, err := rt.providers.Get(agent.ProviderRef())
			if err != nil {
				t.Fatal(err)
			}
			hub := rt.deciderHub()
			cfg := hub.Config()
			ac := cfg.Authorities[authModelRouter]
			ac.Threshold = test.threshold
			cfg.Authorities[authModelRouter] = ac
			if _, err := hub.Update(cfg); err != nil {
				t.Fatal(err)
			}
			stub.set(func(stub *decisionStub) { stub.choice["route"] = questionKey(1) })
			if test.pin {
				if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
					state.Route = &db.DecisionRoute{Provider: "b-native", Model: "claude-opus-4-1", Pinned: true}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			req := providers.Request{Model: "claude-sonnet-4-5", Messages: []providers.Message{{Role: providers.RoleUser, Text: "Implement this task"}}}
			if test.resume {
				req.ResumeSessionID = "resumed-cli-session"
			}
			routed, _, actual := rt.routeDecisionTurn(ctx, agent, provider, req)
			if routed.Model != test.wantModel || actual.Model != test.wantModel || stub.calls() != test.wantCalls {
				t.Fatalf("route model=%s req=%s calls=%d; want %s/%d", routed.Model, actual.Model, stub.calls(), test.wantModel, test.wantCalls)
			}
			if test.wantCalls > 0 {
				stub.mu.Lock()
				body := stub.bodies[0]
				stub.mu.Unlock()
				for _, excluded := range []string{"claude-opus-disabled", "claude-opus-unavailable", "cli/opus"} {
					if strings.Contains(body, excluded) {
						t.Errorf("ineligible model reached judge candidate list: %s", excluded)
					}
				}
			}
		})
	}
}

func selectWorkflowCapabilities(rt *Runtime, stub *decisionStub, ctx context.Context, agent db.Agent, req providers.Request, skillSlug, toolName string) {
	items := []db.DecisionItem{}
	allowed := rt.skills.AllowedFor(agent.Skills)
	for _, skill := range rt.skills.Search("", 0) {
		if allowed[skill.Slug] && !skill.Archived {
			items = append(items, db.DecisionItem{Key: "skill:" + skill.Slug, Kind: "skill", Label: policyText(skill.Slug+" — "+skill.Description+" "+skill.WhenToUse, 400)})
		}
	}
	for _, tool := range rt.LazyToolCatalog(ctx, agent) {
		items = append(items, db.DecisionItem{Key: "tool:" + tool.Name, Kind: "tool", Label: policyText(tool.Name+" — "+tool.Description, 400)})
	}
	items = rankSetupCandidates(items, latestPolicyPrompt(req), rt.policyConfig(authSessionSetup).CandidateLimit)
	stub.set(func(stub *decisionStub) {
		for i, item := range items {
			stub.noul[questionKey(i)] = 0.01
			if item.Key == "skill:"+skillSlug || toolName != "" && item.Key == "tool:"+toolName {
				stub.noul[questionKey(i)] = 0.99
			}
		}
	})
}

func TestDecisionSetupReachesFirstNativeRequestThroughBothIngresses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "traced"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authSessionSetup: decider.ModeOn})
			if _, err := rt.Skills().Create("native-capability", skills.SkillInput{Name: "Native capability", Body: "Loaded native capability instructions"}); err != nil {
				t.Fatal(err)
			}
			agent.Skills = []string{"native-capability"}
			lazy := rt.LazyToolCatalog(ctx, agent)
			if len(lazy) == 0 {
				t.Fatal("fixture has no lazy tools")
			}
			toolName := lazy[0].Name
			req := providers.Request{System: "Baseline instructions", Model: agent.Model, Messages: []providers.Message{{Role: providers.RoleUser, Text: "native capability " + toolName}}}
			selectWorkflowCapabilities(rt, stub, ctx, agent, req, "native-capability", toolName)
			provider := &fakeProvider{script: []scriptedResp{{text: "Done", stop: providers.StopEndTurn}}}
			var err error
			if stream {
				_, _, err = rt.CompleteWithToolsStream(ctx, agent, provider, req, false, func(TurnStep) {})
			} else {
				_, _, err = rt.CompleteWithToolsTraced(ctx, agent, provider, req, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].System, "Loaded native capability instructions") || !requestHasTool(provider.requests[0], toolName) || stub.calls() != 1 {
				t.Fatalf("first native request missed prepared capability: requests=%d judge=%d", len(provider.requests), stub.calls())
			}
			state, err := rt.db.ReadSessionDecisions(ctx, SessionIDFrom(ctx))
			if err != nil || state.SkillBodies["native-capability"] == "" {
				t.Fatalf("loaded instruction snapshot was not persisted: %+v, %v", state, err)
			}
		})
	}
}

func TestDecisionAuxiliaryIngressDoesNotPrepareSessionWorkflows(t *testing.T) {
	rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authSessionSetup: decider.ModeOn, authModelRouter: decider.ModeOn, authContextReminder: decider.ModeOn})
	if _, err := rt.Skills().Create("auxiliary-capability", skills.SkillInput{Name: "Auxiliary capability", Body: "Do not load into titles"}); err != nil {
		t.Fatal(err)
	}
	agent.Skills = []string{"auxiliary-capability"}
	agent.MCPEnabled = false
	if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
		state.CompactCount = 10
		state.Memories = []db.DecisionMemory{{Key: "remember", Text: "Do not inject into titles"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{script: []scriptedResp{{text: "Title", stop: providers.StopEndTurn}}}
	_, _, err := rt.CompleteWithToolsTraced(WithCallKind(ctx, KindTitle), agent, provider, providers.Request{Model: agent.Model, Messages: []providers.Message{{Role: providers.RoleUser, Text: "Create a title"}}}, false)
	if err != nil || stub.calls() != 0 || len(provider.requests) != 1 || provider.requests[0].SystemDynamic != "" || strings.Contains(provider.requests[0].System, "Do not load into titles") {
		t.Fatalf("auxiliary completion ran session preparation: err=%v judge=%d requests=%+v", err, stub.calls(), provider.requests)
	}
}

func TestDecisionWorkerReviewPerspectivesReachCanonicalNotification(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "selected perspectives"
		if failure {
			name = "judge failure preserves terminal note"
		}
		t.Run(name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authWorkerReview: decider.ModeOn})
			worker, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID})
			if err != nil {
				t.Fatal(err)
			}
			stub.set(func(stub *decisionStub) {
				stub.noul[questionKey(0)] = 0.99
				if failure {
					stub.status = 400
				}
			})
			// Pause automatic coordinator synthesis, while exercising the complete
			// durable notification and terminal-worker archive path.
			slot := rt.coordSlotFor(SessionIDFrom(ctx))
			slot.mu.Lock()
			slot.stallHalted = true
			slot.mu.Unlock()
			note := "<task-notification>Terminal worker result</task-notification>"
			if err := rt.notifyCoordinator(SessionIDFrom(ctx), note, false, nil, worker.ID); err != nil {
				t.Fatal(err)
			}
			messages, err := rt.db.ListMessages(ctx, SessionIDFrom(ctx))
			if err != nil || len(messages) != 1 || messages[0].Origin != "worker-note" || !strings.HasPrefix(messages[0].Text, note) {
				t.Fatalf("terminal result was not delivered canonically: %+v, %v", messages, err)
			}
			if failure && messages[0].Text != note || !failure && !strings.Contains(messages[0].Text, "Verify claimed results") {
				t.Fatalf("stored notification lost result or selected perspectives: %q", messages[0].Text)
			}
			storedWorker, err := rt.db.GetSession(ctx, worker.ID)
			if err != nil || storedWorker.State != "archived" {
				t.Fatalf("terminal delivery failed to archive worker: %+v, %v", storedWorker, err)
			}
		})
	}
}

func TestDecisionSetupBudgetsAndShadowToActiveTransition(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		name := "observed setup does not block activation"
		if oversized {
			name = "oversized instructions do not count as loaded"
		}
		t.Run(name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authSessionSetup: decider.ModeShadow})
			body := "Snapshot skill instructions"
			if oversized {
				body = strings.Repeat("large skill instruction ", 300)
			}
			if _, err := rt.Skills().Create("budget-capability", skills.SkillInput{Name: "Budget capability", Body: body}); err != nil {
				t.Fatal(err)
			}
			agent.Skills = []string{"budget-capability"}
			req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "budget capability"}}}
			selectWorkflowCapabilities(rt, stub, ctx, agent, req, "budget-capability", "")
			rt.prepareDecisionSetup(ctx, agent, &req)
			waitBackground(rt)
			if req.System != "" {
				t.Fatal("shadow setup injected instructions")
			}
			hub := rt.deciderHub()
			cfg := hub.Config()
			ac := cfg.Authorities[authSessionSetup]
			ac.Mode, ac.ContextBudget = decider.ModeOn, 1024
			cfg.Authorities[authSessionSetup] = ac
			if _, err := hub.Update(cfg); err != nil {
				t.Fatal(err)
			}
			rt.prepareDecisionSetup(ctx, agent, &req)
			state, err := rt.db.ReadSessionDecisions(ctx, SessionIDFrom(ctx))
			if err != nil || stub.calls() != 2 || len(state.Entries) != 2 || state.Entries[1].Mode != "on" {
				t.Fatalf("shadow entry blocked active setup: %+v, %v, calls=%d", state, err, stub.calls())
			}
			if oversized {
				if len(state.SelectedSkills) != 0 || len(state.SkillBodies) != 0 || req.System != "" || state.Entries[1].Applied != "none" {
					t.Fatalf("oversized skill incorrectly recorded as applied: %+v", state)
				}
				return
			}
			if len(state.SelectedSkills) != 1 || !strings.Contains(req.System, body) {
				t.Fatal("active setup did not load the previously observed skill")
			}
			if _, err := rt.Skills().Update("budget-capability", skills.SkillInput{Name: "Budget capability", Body: "Edited skill instructions"}); err != nil {
				t.Fatal(err)
			}
			req = providers.Request{}
			rt.prepareDecisionSetup(ctx, agent, &req)
			if !strings.Contains(req.System, body) || strings.Contains(req.System, "Edited skill instructions") {
				t.Fatal("catalog edit changed the frozen session instruction snapshot")
			}
			agent.Skills = nil
			req = providers.Request{}
			rt.prepareDecisionSetup(ctx, agent, &req)
			if req.System != "" {
				t.Fatal("frozen skill snapshot bypassed revoked access")
			}
		})
	}
}

func TestDecisionRouterRespectsPinChangedWhileJudgeRuns(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	t.Cleanup(func() { drainSpawns(t, rt) })
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Pinned route", Provider: "anthropic", ProviderInstanceID: "a-native", Model: "claude-sonnet-4-5"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	ctx = WithSessionID(ctx, session.ID)
	rt.providers.SetInstances([]providers.Instance{
		{ID: "a-native", KindID: "anthropic", Enabled: true, Models: "claude-sonnet-4-5", Values: map[string]string{providers.FieldKeyAPIKey: "fixture"}},
		{ID: "b-native", KindID: "anthropic", Enabled: true, Models: "claude-opus-4-1", Values: map[string]string{providers.FieldKeyAPIKey: "fixture"}},
	})
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		close(started)
		<-release
		_, _ = io.WriteString(w, `{"answers":{"route":{"type":"choice","choice":"candidate_1","probabilities":{"candidate_1":0.99},"confidence":0.99}}}`)
	}))
	defer server.Close()
	hub := decider.NewHub(decider.HubOptions{Source: stubSource{url: server.URL}})
	cfg := hub.Config()
	cfg.Enabled = true
	ac := cfg.Authorities[authModelRouter]
	ac.Mode = decider.ModeOn
	cfg.Authorities[authModelRouter] = ac
	if _, err := hub.Update(cfg); err != nil {
		t.Fatal(err)
	}
	tun.SetDecider(hub)
	result := make(chan db.Agent, 1)
	go func() { result <- rt.RouteSessionAgent(ctx, agent, "Implement this task") }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("router never reached the judge")
	}
	if err := rt.db.UpdateSessionDecisions(ctx, session.ID, func(state *db.SessionDecisions) error {
		state.Route = &db.DecisionRoute{Provider: "a-native", Model: "claude-sonnet-4-5", Pinned: true}
		return nil
	}); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	var routed db.Agent
	select {
	case routed = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("router did not finish after the judge returned")
	}
	state, err := rt.db.ReadSessionDecisions(ctx, session.ID)
	if err != nil || routed.Model != "claude-sonnet-4-5" || state.Route == nil || !state.Route.Pinned || state.Route.Provider != "a-native" {
		t.Fatalf("concurrent pin was overwritten: routed=%+v state=%+v err=%v", routed, state.Route, err)
	}
}

func TestDecisionRouterCLIStaysWithinTheCurrentInstance(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, pinnedOtherInstance := range []bool{false, true} {
		name := "judge candidates stay in one CLI instance"
		if pinnedOtherInstance {
			name = "persisted pin cannot cross CLI instances"
		}
		t.Run(name, func(t *testing.T) {
			rt, stub, ctx, agent := decisionWorkflowFixture(t, map[string]decider.Mode{authModelRouter: decider.ModeOn})
			rt.providers.SetInstances([]providers.Instance{
				{ID: "cli-a", KindID: "claude-cli", Enabled: true, Models: "sonnet,opus", Values: map[string]string{providers.FieldKeyCLIPath: executable}},
				{ID: "cli-b", KindID: "claude-cli", Enabled: true, Models: "haiku", Values: map[string]string{providers.FieldKeyCLIPath: executable}},
			})
			agent.Provider, agent.ProviderInstanceID, agent.Model = "claude-cli", "cli-a", "sonnet"
			stub.set(func(stub *decisionStub) { stub.choice["route"] = questionKey(1) })
			if pinnedOtherInstance {
				if err := rt.db.UpdateSessionDecisions(ctx, SessionIDFrom(ctx), func(state *db.SessionDecisions) error {
					state.Route = &db.DecisionRoute{Provider: "cli-b", Model: "haiku", Pinned: true}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			routed := rt.RouteSessionAgent(ctx, agent, "Implement this task")
			if routed.ProviderRef() != "cli-a" {
				t.Fatalf("CLI routing changed the provider instance: %s", routed.ProviderRef())
			}
			if pinnedOtherInstance {
				if routed.Model != "sonnet" || stub.calls() != 0 {
					t.Fatal("incompatible persisted pin changed the CLI route")
				}
				return
			}
			if routed.Model != "opus" || stub.calls() != 1 {
				t.Fatalf("eligible model in the current CLI instance was not selected: model=%s calls=%d", routed.Model, stub.calls())
			}
			stub.mu.Lock()
			body := stub.bodies[0]
			stub.mu.Unlock()
			if strings.Contains(body, "cli-b") || strings.Contains(body, "haiku") {
				t.Fatal("a different CLI instance reached the judge candidate list")
			}
		})
	}
}
