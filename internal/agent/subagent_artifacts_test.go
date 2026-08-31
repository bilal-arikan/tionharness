package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestChildArtifactSinkIsBoundToTheChildSession: the sink is inherited from the
// caller and the tool loop only installs a fallback when none is present, so
// without an explicit re-bind a subagent's artifact is filed under the CALLER —
// the delegated work loses its provenance exactly where it matters most.
func TestChildArtifactSinkIsBoundToTheChildSession(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	parentAgent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic"})
	childAgent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})
	parentID, childID := subagentChild(t, rt, parentAgent.ID, "completed")

	// Caller's context carries the PARENT's sink, exactly as the tool loop leaves it;
	// the child run re-binds on top of it. Re-binding must win — the tool loop's own
	// fallback is skipped whenever a sink is already present.
	callerCtx := tools.WithArtifacts(ctx, rt.NewArtifactSink(parentID, parentAgent.ID))
	childCtx := tools.WithArtifacts(callerCtx, rt.NewArtifactSink(childID, childAgent.ID))
	if !tools.HasArtifactSink(childCtx) {
		t.Fatal("expected an artifact sink on the child context")
	}

	sink := rt.NewArtifactSink(childID, childAgent.ID)
	ref, err := sink.CreateArtifact(childCtx, tools.CreateArtifactSpec{
		Title: "Findings", Kind: "markdown", Content: "# result",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	got, err := rt.db.GetArtifact(ctx, ref.ID)
	if err != nil {
		t.Fatalf("read artifact back: %v", err)
	}
	if got.SessionID != childID {
		t.Fatalf("artifact must be owned by the child run, got session %s", got.SessionID)
	}
	if got.AgentID != childAgent.ID {
		t.Fatalf("artifact must be attributed to the subagent, got agent %s", got.AgentID)
	}
	// The parent's own session must stay clean — the point of the re-bind.
	parentRows, err := rt.db.ListArtifacts(ctx, parentID)
	if err != nil {
		t.Fatalf("list parent artifacts: %v", err)
	}
	if len(parentRows) != 0 {
		t.Fatalf("parent session should own no artifacts, got %d", len(parentRows))
	}
}

// TestCollectChildArtifactsReturnsReferencesOnly: references travel back, not
// bodies — inlining a produced document would undo the whole reason to delegate.
func TestCollectChildArtifactsReturnsReferencesOnly(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})
	_, childID := subagentChild(t, rt, a.ID, "completed")

	if got := rt.collectChildArtifacts(ctx, childID); len(got) != 0 {
		t.Fatalf("a run that produced nothing returns no references, got %d", len(got))
	}

	sink := rt.NewArtifactSink(childID, a.ID)
	body := strings.Repeat("x", 4096)
	if _, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{
		Title: "Report", Kind: "markdown", Content: body,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	refs := rt.collectChildArtifacts(ctx, childID)
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Title != "Report" || refs[0].Kind != "markdown" || refs[0].ID == "" {
		t.Fatalf("reference should carry id/title/kind, got %+v", refs[0])
	}
}

// TestRunSubagentResultListsArtifacts: the caller is told what the run produced,
// with ids it can pass to read_artifact — otherwise the artifact exists but is
// invisible to the only party that asked for the work.
func TestRunSubagentResultListsArtifacts(t *testing.T) {
	out, err := tools.FormatRunAgentResult(tools.RunAgentResult{
		AgentName: "Helper",
		Reply:     "done",
		Artifacts: []tools.SubagentArtifact{{ID: "ART7", Title: "Report", Kind: "markdown"}},
	})
	if err != nil {
		t.Fatalf("format result: %v", err)
	}
	if !strings.Contains(out, "ART7") || !strings.Contains(out, "read_artifact") {
		t.Fatalf("result must name the artifact and how to read it: %s", out)
	}

	plain, err := tools.FormatRunAgentResult(tools.RunAgentResult{AgentName: "Helper", Reply: "done"})
	if err != nil {
		t.Fatalf("format plain result: %v", err)
	}
	if strings.Contains(plain, "Artifacts it produced") {
		t.Fatalf("a run with no artifacts must not grow a section: %s", plain)
	}
}
