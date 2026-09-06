package agent

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

func evolverFixture(t *testing.T) (*Runtime, db.Goal, db.Agent) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	r := &Runtime{db: database, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	dev, _ := database.CreateAgent(ctx, db.Agent{Name: "Dev", Soul: "be brief", Model: "sonnet", ThinkingLevel: "high"})
	g, _ := database.CreateGoal(ctx, db.Goal{
		Name: "Ucuz Dev", Status: db.GoalStatusActive,
		Scope:      db.GoalScope{Agents: []string{dev.ID}},
		Primary:    db.GoalMetric{Metric: "usage.costUSDPerSession", Direction: "min"},
		Guardrails: []db.GoalGuardrail{{Metric: "session.errorTurnsRatio", Max: fptr(0.2)}},
	}, db.GoalByUser, "")
	return r, g, dev
}

func fptr(v float64) *float64 { return &v }

func rawFor(agentID string) goals.RawProposal {
	return goals.RawProposal{
		Surface: "agent", EntityID: agentID, Field: "thinkingLevel", Action: "set", Value: "low",
		Title: "Düşünme seviyesini düşür", Rationale: "kısa görevler", Evidence: "$1.4 over 8 sessions",
		ExpectedMetric: "usage.costUSDPerSession", ExpectedDelta: -0.4, Severity: "med",
	}
}

// TestFileEvolverProposals: filing applies the cap, dedupes within a pass,
// respects a user dismissal, escalates a proposal that keeps returning open
// and then silences it, and flags low-confidence passes in the title.
func TestFileEvolverProposals(t *testing.T) {
	ctx := context.Background()
	r, g, dev := evolverFixture(t)
	store, err := insight.OpenFindingStore(r.db.Root())
	if err != nil {
		t.Fatal(err)
	}
	in := goals.FitnessInputs{CurrentHash: "h1"}
	state := db.EvolutionGoalState{Repeats: map[string]int{}, Escalated: map[string]bool{}}

	// Pass 1: four raws → cap 3, one duplicate, one invisible field dropped.
	raws := []goals.RawProposal{rawFor(dev.ID), rawFor(dev.ID)}
	soul := rawFor(dev.ID)
	soul.Field, soul.Value = "soul", "be brief and cheap"
	model := rawFor(dev.ID)
	model.Field, model.Value = "model", "haiku"
	perm := rawFor(dev.ID)
	perm.Field, perm.Value = "permissionMode", "auto"
	raws = append(raws, soul, model, perm)
	res := EvolutionResult{LowConfidence: true}
	r.fileEvolverProposals(ctx, g, raws, nil, in, &state, store, nil, &res, 100)
	if len(res.Proposals) != 3 || res.Dropped != 2 {
		t.Fatalf("pass 1: proposals=%d dropped=%d reasons=%v", len(res.Proposals), res.Dropped, res.DropReasons)
	}
	if !strings.HasPrefix(res.Proposals[0].Title, "[düşük güven]") || res.Proposals[0].Evolution == nil || res.Proposals[0].Evolution.SnapshotHash != "h1" || res.Proposals[0].Channel != insight.ChannelEvolution {
		t.Fatalf("finding shape: %+v", res.Proposals[0])
	}
	sig := goals.Signature(*res.Proposals[0].Evolution)

	// The user dismisses the thinking proposal: it is never re-filed.
	if _, err := store.SetStatus(res.Proposals[0].ID, insight.StatusDismissed, 101, nil); err != nil {
		t.Fatal(err)
	}
	previous := store.List(evolverLensID, insight.ChannelEvolution)
	res = EvolutionResult{}
	r.fileEvolverProposals(ctx, g, []goals.RawProposal{rawFor(dev.ID)}, nil, in, &state, store, previous, &res, 102)
	if len(res.Proposals) != 0 || res.Dropped != 1 || !strings.Contains(res.DropReasons[0], "dismissed") {
		t.Fatalf("dismissed proposal re-filed: %+v", res)
	}

	// The soul proposal stays open and returns pass after pass: repeats climb,
	// the third repeat files ONE escalation and later passes drop it.
	for i := 1; i <= goals.EscalateAfterRepeats+1; i++ {
		previous = store.List(evolverLensID, insight.ChannelEvolution)
		res = EvolutionResult{}
		r.fileEvolverProposals(ctx, g, []goals.RawProposal{soul}, nil, in, &state, store, previous, &res, int64(110+i))
		soulSig := goals.Signature(insight.EvolutionProposal{GoalID: g.ID, Surface: "agent", EntityID: dev.ID, Field: "soul", Action: "set"})
		switch {
		case i < goals.EscalateAfterRepeats:
			if len(res.Proposals) != 1 || res.Proposals[0].Evolution.Kind != "change" || state.Repeats[soulSig] != i {
				t.Fatalf("repeat %d: %+v repeats=%v", i, res.Proposals, state.Repeats)
			}
		case i == goals.EscalateAfterRepeats:
			if len(res.Proposals) != 1 || res.Proposals[0].Evolution.Kind != "escalation" || !state.Escalated[soulSig] || !strings.HasPrefix(res.Proposals[0].Title, "İnsan kararı") {
				t.Fatalf("escalation expected on repeat %d: %+v", i, res.Proposals)
			}
		default:
			if len(res.Proposals) != 0 || !strings.Contains(res.DropReasons[0], "escalated") {
				t.Fatalf("after escalation must drop: %+v", res)
			}
		}
	}
	// Distinct sig for the escalation card; original card kept its occurrences.
	all := store.List(evolverLensID, insight.ChannelEvolution)
	esc, orig := 0, 0
	for _, f := range all {
		if f.Evolution != nil && f.Evolution.Kind == "escalation" {
			esc++
		}
		if strings.EqualFold(f.Signature, goals.Signature(insight.EvolutionProposal{GoalID: g.ID, Surface: "agent", EntityID: dev.ID, Field: "soul", Action: "set"})) {
			orig = f.Occurrences
		}
	}
	if esc != 1 || orig < 2 {
		t.Fatalf("escalations=%d original occurrences=%d", esc, orig)
	}
	_ = sig
}

// TestEvolverPromptAndSurfaces: the prompt carries the goal, the fitness
// series, the in-scope surfaces, the worst runs, earlier proposals and the
// editable-surface list; invisible fields never appear.
func TestEvolverPromptAndSurfaces(t *testing.T) {
	ctx := context.Background()
	r, g, dev := evolverFixture(t)
	other, _ := r.db.CreateAgent(ctx, db.Agent{Name: "Other", Soul: "x"})
	_, _ = r.db.CreateAutomation(ctx, db.Automation{Name: "nightly", Enabled: true, TriggerKind: "tag", TriggerTag: "t", CooldownSec: 60})
	sc := r.evolverSurfaces(ctx, g)
	if len(sc.Agents) != 1 || !strings.Contains(sc.Agents[0], dev.ID) || strings.Contains(strings.Join(sc.Agents, ""), other.ID) {
		t.Fatalf("agent scope not honoured: %v", sc.Agents)
	}
	if len(sc.Automations) != 1 || !strings.Contains(sc.Automations[0], "cooldownSec=60") {
		t.Fatalf("automations: %v", sc.Automations)
	}
	fit := goals.GoalFitness{Sessions: 6, Direction: "min",
		Primary:    goals.MetricValue{Metric: "usage.costUSDPerSession", Value: fptr(1.4), N: 6, Unit: "usd", Available: true},
		Guardrails: []goals.GuardrailStatus{{MetricValue: goals.MetricValue{Metric: "session.errorTurnsRatio", Value: fptr(0.5), N: 6, Unit: "ratio", Available: true}, Violated: true}},
		BySnapshot: []goals.SnapshotFitness{{Hash: "c2c8da07aaaa", Sessions: 2, Primary: goals.MetricValue{Metric: "usage.costUSDPerSession", Value: fptr(0.9), N: 2, Unit: "usd", Available: true}},
			{Hash: "cb72d0f4bbbb", Sessions: 4, Current: true, Primary: goals.MetricValue{Metric: "usage.costUSDPerSession", Value: fptr(1.6), N: 4, Unit: "usd", Available: true},
				Changes: []goals.SnapshotChange{{Surface: "agent", Entity: dev.ID, Field: "thinkingLevel", Before: "low", After: "high"}}}},
	}
	worst := []db.TrajectoryIndexEntry{{ID: "RTA7", RootSessionID: "SES7", TemplateRef: "code-review@2", Status: db.TrajStatusFailed, Summary: &db.TrajectorySummary{CostUSD: 3.2, Sessions: 4, FailedSess: 2}}}
	previous := []insight.Finding{{Title: "eski öneri", Status: insight.StatusDismissed, Evolution: &insight.EvolutionProposal{GoalID: g.ID, Surface: "agent", Field: "model", Action: "set", EntityID: dev.ID}}}
	p := evolverUserPrompt(g, fit, sc, worst, previous, "Türkçe")
	for _, want := range []string{
		"Goal " + g.ID, "usage.costUSDPerSession = 1.4", "VIOLATED", "cb72d0f4 (current)", "changed: agent " + dev.ID + " thinkingLevel: low → high",
		"RTA7 code-review@2 [failed]", "[dismissed] eski öneri", "## Editable surfaces", "thinkingLevel", "Reply language", "Türkçe",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, forbidden := range []string{"permissionMode", "inboundPolicy", "hook"} {
		if strings.Contains(p, forbidden) {
			t.Errorf("prompt must not expose %q", forbidden)
		}
	}
}

// TestRunGoalEvolverGates: without sessions or under the threshold on an
// automatic trigger the pass is skipped and recorded, never calling a model.
func TestRunGoalEvolverGates(t *testing.T) {
	ctx := context.Background()
	r, g, dev := evolverFixture(t)
	res, err := r.RunGoalEvolver(ctx, g.ID, evolverTrigRuns)
	if err != nil || res.Ran || res.Skipped != "no sessions in scope yet" {
		t.Fatalf("no sessions: %+v %v", res, err)
	}
	_, _ = r.db.CreateSession(ctx, db.Session{AgentID: dev.ID})
	res, _ = r.RunGoalEvolver(ctx, g.ID, evolverTrigRuns)
	if res.Ran || !strings.Contains(res.Skipped, "only 1 scoped sessions") {
		t.Fatalf("threshold: %+v", res)
	}
	st, _ := r.db.GetEvolutionState(ctx)
	if st.Goals[g.ID].SessionsSeen != 1 || st.Goals[g.ID].Skipped == "" {
		t.Fatalf("state not recorded: %+v", st.Goals[g.ID])
	}
	if _, err := r.RunGoalEvolver(ctx, "GOL99", evolverTrigManual); err == nil {
		t.Fatal("unknown goal must error")
	}
	if err := r.db.DeleteEvolutionGoalState(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	st, _ = r.db.GetEvolutionState(ctx)
	if _, ok := st.Goals[g.ID]; ok {
		t.Fatal("state must be deleted")
	}
}
