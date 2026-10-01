package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func policyFoldFixture(t *testing.T) (*db.DB, db.Agent, db.Session, []db.Message) {
	t.Helper()
	d := openFoldTestDB(t)
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "Fold", Provider: "anthropic", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(ctx, db.Session{AgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"obsolete fragment", "essential exact constraint", "normal conversation", "assistant followup", "latest user request", "latest assistant response"} {
		role := providers.RoleUser
		if i%2 == 1 {
			role = providers.RoleAssistant
		}
		if _, err := d.AddMessage(ctx, db.Message{SessionID: session.ID, Role: role, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := d.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d, agent, session, history
}

func TestFoldPolicyAppliesToManualAndRollingSummaries(t *testing.T) {
	for _, manual := range []bool{true, false} {
		name := "rolling"
		if manual {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			d, agent, session, history := policyFoldFixture(t)
			manager := NewManager()
			manager.SetLimits(10, 2)
			provider := &captureProvider{name: "fixture"}
			finished, reviewed := 0, 0
			ctx := WithFoldPolicy(context.Background(), func(ctx context.Context, existing string, segments []FoldSegment) FoldPlan {
				reviewed++
				if FoldSessionID(ctx) != session.ID || len(segments) != 4 || segments[0].SourceID != history[0].ID {
					t.Fatalf("fold provenance mismatch: session=%s segments=%+v", FoldSessionID(ctx), segments)
				}
				var rendered strings.Builder
				for _, segment := range segments[1:] {
					rendered.WriteString(segment.Rendered)
				}
				return FoldPlan{Rendered: rendered.String(), Protected: "essential exact constraint"}
			}, func(ctx context.Context) {
				finished++
				if FoldSessionID(ctx) != session.ID {
					t.Fatal("finished hook lost the folded session identity")
				}
			})
			if manual {
				if _, _, err := manager.ForceCompact(ctx, d, provider, session, agent, history); err != nil {
					t.Fatal(err)
				}
			} else {
				prepared, err := manager.Prepare(ctx, d, provider, session, agent, history)
				if err != nil || !prepared.Compacted {
					t.Fatalf("rolling fold: %+v, %v", prepared, err)
				}
			}
			if reviewed != 1 || finished != 1 || provider.hits != 1 {
				t.Fatalf("hook invocation counts: reviewed=%d finished=%d provider=%d", reviewed, finished, provider.hits)
			}
			input := provider.req.Messages[0].Text
			if strings.Contains(input, "obsolete fragment") || !strings.Contains(input, "normal conversation") {
				t.Fatalf("policy did not filter the real summarizer input: %q", input)
			}
			stored, err := d.GetSession(context.Background(), session.ID)
			if err != nil || !strings.HasSuffix(stored.Summary, "essential exact constraint") {
				t.Fatalf("verbatim protection was not persisted: %+v, %v", stored, err)
			}
			canonical, err := d.ListMessages(context.Background(), session.ID)
			if err != nil || len(canonical) != 6 || canonical[0].Text != "obsolete fragment" {
				t.Fatalf("fold policy mutated canonical history: %+v, %v", canonical, err)
			}
		})
	}
}

func TestFoldPolicyReactiveUsesFilteredInputAndProtectedOutput(t *testing.T) {
	d, agent, _, _ := policyFoldFixture(t)
	provider := &captureProvider{name: "fixture"}
	finished := 0
	ctx := WithFoldPolicy(context.Background(), func(_ context.Context, _ string, segments []FoldSegment) FoldPlan {
		if len(segments) != 3 || segments[0].SourceID != "" {
			t.Fatalf("reactive segments must be transient: %+v", segments)
		}
		return FoldPlan{Rendered: "filtered input only", Protected: "protected verbatim"}
	}, func(context.Context) { finished++ })
	messages := []providers.Message{
		{Role: providers.RoleUser, Text: "obsolete raw message"},
		{Role: providers.RoleAssistant, Text: "old assistant"},
		{Role: providers.RoleUser, Text: "previous user"},
		{Role: providers.RoleAssistant, Text: "recent assistant"},
		{Role: providers.RoleUser, Text: "current user"},
	}
	out, _, ok, err := CompactInFlightMessages(ctx, d, provider, agent, messages, 2)
	if err != nil || !ok || finished != 1 || len(out) != 3 {
		t.Fatalf("reactive fold: ok=%v finished=%d out=%+v err=%v", ok, finished, out, err)
	}
	if !strings.Contains(provider.req.Messages[0].Text, "filtered input only") || strings.Contains(provider.req.Messages[0].Text, "obsolete raw message") || !strings.HasSuffix(out[0].Text, "protected verbatim") {
		t.Fatal("reactive policy did not affect actual summarizer input and output")
	}
	if messages[0].Text != "obsolete raw message" || out[2].Text != "current user" {
		t.Fatal("reactive policy corrupted source or retained tail")
	}
}

func TestFoldPolicyFailedSummaryNeverFinishes(t *testing.T) {
	d, agent, session, history := policyFoldFixture(t)
	manager := NewManager()
	manager.SetLimits(10, 2)
	provider := &countingErrorProvider{}
	finished := 0
	ctx := WithFoldPolicy(context.Background(), nil, func(context.Context) { finished++ })
	if _, _, err := manager.ForceCompact(ctx, d, provider, session, agent, history); err == nil {
		t.Fatal("failing manual fold unexpectedly succeeded")
	}
	if prepared, err := manager.Prepare(ctx, d, provider, session, agent, history); err != nil || !prepared.FoldFailed {
		t.Fatalf("failed rolling fold did not preserve fallback: %+v, %v", prepared, err)
	}
	messages := []providers.Message{{Role: providers.RoleUser, Text: "old"}, {Role: providers.RoleAssistant, Text: "old response"}, {Role: providers.RoleUser, Text: "question"}, {Role: providers.RoleAssistant, Text: "recent"}, {Role: providers.RoleUser, Text: "current"}}
	if _, _, ok, err := CompactInFlightMessages(ctx, d, provider, agent, messages, 2); err == nil || ok {
		t.Fatal("failing reactive fold unexpectedly succeeded")
	}
	if finished != 0 {
		t.Fatalf("failed fold counted as completed: %d", finished)
	}
	stored, err := d.GetSession(context.Background(), session.ID)
	if err != nil || stored.Summary != "" || stored.SummaryMsgCount != 0 {
		t.Fatalf("failure committed a session summary: %+v, %v", stored, err)
	}
}
