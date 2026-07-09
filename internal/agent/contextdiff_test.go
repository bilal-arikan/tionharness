package agent

import (
	"context"
	"strings"
	"testing"
)

// TestDiffSystemPrefixParagraphs: a paragraph-level diff surfaces added/removed
// blocks, self-labelled by their first line, and returns nil when identical.
func TestDiffSystemPrefixParagraphs(t *testing.T) {
	if c := diffSystemPrefix("SAME", "SAME"); c != nil {
		t.Fatalf("identical prefixes must diff to nil, got %+v", c)
	}
	old := "# Persona\nyou are helpful"
	nw := "# Persona\nyou are helpful\n\n# Workspace Instructions\nbe terse"
	c := diffSystemPrefix(old, nw)
	if c == nil || c.Added != 1 || c.Removed != 0 {
		t.Fatalf("expected 1 added block, got %+v", c)
	}
	if len(c.Areas) != 1 || c.Areas[0].Kind != ContextAdded || !strings.Contains(c.Areas[0].Label, "Workspace Instructions") {
		t.Fatalf("added area must be labelled by its header, got %+v", c.Areas)
	}
}

// TestDiffSystemPrefixModifiedBlock: a changed paragraph reads as one removal
// (old form) plus one addition (new form).
func TestDiffSystemPrefixModifiedBlock(t *testing.T) {
	c := diffSystemPrefix("# A\nold body", "# A\nnew body")
	if c == nil || c.Added != 1 || c.Removed != 1 {
		t.Fatalf("modified block must be remove+add, got %+v", c)
	}
}

// TestDiffToolNames: added/removed tool names are itemised; a pure name-set
// match returns nil.
func TestDiffToolNames(t *testing.T) {
	if c := diffToolNames([]string{"a", "b"}, []string{"a", "b"}); c != nil {
		t.Fatalf("identical name sets must diff to nil, got %+v", c)
	}
	c := diffToolNames([]string{"a", "b"}, []string{"a", "c"})
	if c == nil || c.Added != 1 || c.Removed != 1 {
		t.Fatalf("expected +c/-b, got %+v", c)
	}
	labels := c.Areas[0].Label + "|" + c.Areas[1].Label
	if !strings.Contains(labels, "Tool: c") || !strings.Contains(labels, "Tool: b") {
		t.Fatalf("tool areas must name the tools, got %q", labels)
	}
}

// TestSuffixNoteMarkerAndCap: the rich note is wrapped in the shared marker (so
// the tool-loop double-append guard works) and caps the listed areas.
func TestSuffixNoteMarkerAndCap(t *testing.T) {
	c := &ContextChange{}
	for i := 0; i < maxNoteAreasListed+3; i++ {
		c.appendArea(ContextArea{Label: "block " + itoa(i), Kind: ContextAdded})
		c.Added++
	}
	note := c.SuffixNote()
	if !strings.Contains(note, suffixNoteMarker) || !strings.HasSuffix(note, "</context_snapshot_note>") {
		t.Fatalf("note must be wrapped in the shared marker: %q", note)
	}
	if !strings.Contains(note, "and 3 more") {
		t.Fatalf("note must cap listed areas with an overflow line: %q", note)
	}
	// An empty change falls back to the generic stale note (never a bare marker).
	if (&ContextChange{}).SuffixNote() != PromptEpochStaleNote {
		t.Fatal("empty change must fall back to the generic stale note")
	}
}

// TestContextChangeOneShotAndNote: while the frozen snapshot lags live state,
// PromptEpochContextNote refreshes every stale turn but ConsumeContextChange
// fires ONCE per drift episode, and reverting the drift re-arms it.
func TestContextChangeOneShotAndNote(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.SetPromptEpoch(true)
	ctx := context.Background()
	sid := "SES_ctxchange"

	// Freeze v1 (no drift yet).
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "# A\nv1" })
	if rt.ConsumeContextChange(sid, epochAgent.ID) != nil {
		t.Fatal("no drift → no context change to consume")
	}

	// Drift: builder now yields a changed block.
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "# A\nv2" })
	note := rt.PromptEpochContextNote(sid, epochAgent.ID)
	if !strings.Contains(note, suffixNoteMarker) || !strings.Contains(note, "What changed") {
		t.Fatalf("stale turn must inject the rich diff note, got %q", note)
	}
	cc := rt.ConsumeContextChange(sid, epochAgent.ID)
	if cc == nil || cc.Removed != 1 || cc.Added != 1 {
		t.Fatalf("first stale turn must yield the change once, got %+v", cc)
	}
	// Second consume in the same episode returns nothing (one-shot).
	if rt.ConsumeContextChange(sid, epochAgent.ID) != nil {
		t.Fatal("context change must fire once per drift episode")
	}
	// ...but the note keeps refreshing while stale.
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "# A\nv2" })
	if !strings.Contains(rt.PromptEpochContextNote(sid, epochAgent.ID), suffixNoteMarker) {
		t.Fatal("note must persist across stale turns")
	}

	// Revert the drift → note clears and the one-shot re-arms.
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "# A\nv1" })
	if rt.PromptEpochContextNote(sid, epochAgent.ID) != "" {
		t.Fatal("reverted drift must clear the note")
	}
	rt.EpochStaticSystem(ctx, sid, epochAgent, false, false, "", func() string { return "# A\nv3" })
	if rt.ConsumeContextChange(sid, epochAgent.ID) == nil {
		t.Fatal("a fresh drift episode must re-arm the one-shot")
	}
}
