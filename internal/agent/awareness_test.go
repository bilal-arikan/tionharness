package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
)

func TestBriefBlockIsFrozenPerSessionAndInvalidated(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	ag, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: ag.ID, Title: "Mine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.notes.Put(notes.Note{Kind: notes.KindLesson, Title: "Quote paths", Body: "b", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred}); err != nil {
		t.Fatal(err)
	}
	first := rt.BriefBlock(ctx, sess, ag)
	for _, want := range []string{"# Session briefing", "Quote paths", "[context meter · brief"} {
		if !strings.Contains(first, want) {
			t.Fatalf("brief lacks %q:\n%s", want, first)
		}
	}
	// The store moves; the frozen brief does not.
	rt.notes.Put(notes.Note{Kind: notes.KindLesson, Title: "Second lesson", Body: "b", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred})
	if again := rt.BriefBlock(ctx, sess, ag); again != first {
		t.Fatal("brief must be byte-stable between adopt points")
	}
	rt.InvalidateBrief(sess.ID)
	if again := rt.BriefBlock(ctx, sess, ag); !strings.Contains(again, "Second lesson") {
		t.Fatal("after invalidation the brief recomposes")
	}
	// A preview never freezes anything.
	if p := rt.BriefPreview(ctx, ag); !strings.Contains(p, "# Session briefing") || !strings.Contains(p, "Second lesson") {
		t.Fatalf("preview: %q", p)
	}
	// The preview froze nothing: the session's brief is still the recomposed one.
	if rt.Awareness().Seen(sess.ID).Brief == nil {
		t.Fatal("the session's own brief must stay recorded")
	}
	if rt.BriefBlock(ctx, db.Session{}, ag) != "" {
		t.Fatal("no session, no brief")
	}
}

func TestTurnBlockMetersAndCarriesLeadSections(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic", Model: "m"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: ag.ID})
	comp := rt.TurnBlock(ctx, sess, ag, false, []awareness.Section{{Key: "clock", Text: "now", Priority: awareness.PriorityPinned}})
	if !strings.HasPrefix(comp.Text, "now") || !strings.Contains(comp.Meter, "turn") || comp.Budget != awareness.DefaultSettings().TurnBudgetBytes {
		t.Fatalf("turn: %+v", comp)
	}
	rt.SetAwarenessSettings(awareness.Settings{Enabled: true, TurnBudgetBytes: 4096})
	if got := rt.TurnBlock(ctx, sess, ag, false, nil).Budget; got != 4096 {
		t.Fatalf("live settings must apply: %d", got)
	}
}

func TestRecordDigestWritesOnceAndForgetPrunes(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic", Model: "m"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: ag.ID, Title: "Digest me"})
	rt.RecordDigest(ctx, sess.ID)
	d, ok := rt.Awareness().LoadDigest(sess.ID)
	if !ok || d.SessionID != sess.ID || d.AgentName != "Ada" {
		t.Fatalf("digest: %+v %v", d, ok)
	}
	if rows := rt.Awareness().RecentDigests(5, ""); len(rows) != 1 {
		t.Fatalf("index: %+v", rows)
	}
	rt.ForgetAwareness(sess.ID)
	if rows := rt.Awareness().AllDigests(0); len(rows) != 0 {
		t.Fatalf("forget must prune: %+v", rows)
	}
}

func TestNoteSupersedeRuleWithoutADecisionModel(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	same := notes.Note{Title: "Use Redis for the cache"}
	if !rt.decideNoteSupersedes(ctx, same, notes.Note{Title: "Use Redis for the cache"}) {
		t.Fatal("identical titles supersede by rule")
	}
	if rt.decideNoteSupersedes(ctx, same, notes.Note{Title: "Use Redis for the session store and queue"}) {
		t.Fatal("a partial overlap is a separate note by rule")
	}
}
