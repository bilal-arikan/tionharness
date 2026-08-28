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

// TestDiffSystemPrefixModifiedBlock: a changed paragraph surfaces as a single
// "modified" area, counted by the lines its diff really changed.
func TestDiffSystemPrefixModifiedBlock(t *testing.T) {
	c := diffSystemPrefix("# A\nold body", "# A\nnew body")
	if c == nil || c.Added != 1 || c.Removed != 1 {
		t.Fatalf("one line swapped must be +1 -1, got %+v", c)
	}
	if len(c.Areas) != 1 || c.Areas[0].Kind != ContextModified {
		t.Fatalf("modified block must collapse into one modified area, got %+v", c.Areas)
	}
}

// TestDiffSystemPrefixModifiedCountsAreLineLevel: the +N/-M headline of a
// modified block reports its real line changes, so a pure deletion never
// advertises an addition the user then hunts for (and vice versa).
func TestDiffSystemPrefixModifiedCountsAreLineLevel(t *testing.T) {
	cases := []struct {
		name             string
		old, nw          string
		wantAdd, wantDel int
	}{
		{"line deleted", "# A\nl1\nl2\nl3", "# A\nl1\nl3", 0, 1},
		{"line added", "# A\nl1\nl3", "# A\nl1\nl2\nl3", 1, 0},
		{"two added one removed", "# A\nl1\nl2", "# A\nl1\nnew1\nnew2", 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := diffSystemPrefix(tc.old, tc.nw)
			if c == nil || len(c.Areas) != 1 || c.Areas[0].Kind != ContextModified {
				t.Fatalf("want one modified area, got %+v", c)
			}
			if c.Added != tc.wantAdd || c.Removed != tc.wantDel {
				t.Fatalf("counts = +%d -%d, want +%d -%d", c.Added, c.Removed, tc.wantAdd, tc.wantDel)
			}
		})
	}
}

// TestDiffSystemPrefixWholeBlockCountsOne: an unpaired paragraph is still one
// unit of change regardless of how many lines it holds.
func TestDiffSystemPrefixWholeBlockCountsOne(t *testing.T) {
	old := "# Keep\nsame\n\n# Gone\nalpha\nbeta\ngamma"
	nw := "# Keep\nsame\n\n# Fresh\nnothing alike\nor here\nor here either"
	c := diffSystemPrefix(old, nw)
	if c == nil || c.Added != 1 || c.Removed != 1 {
		t.Fatalf("one block gained and one lost must be +1 -1, got %+v", c)
	}
}

// TestDiffSystemPrefixModifiedCountsIgnoreAreaCap: the counters are taken from
// the untrimmed diff, so a block far larger than maxAreaLines still reports
// every changed line even though its rendered body is capped.
func TestDiffSystemPrefixModifiedCountsIgnoreAreaCap(t *testing.T) {
	n := maxAreaLines * 3
	var oldB, newB strings.Builder
	oldB.WriteString("# Big")
	newB.WriteString("# Big")
	for i := 0; i < n; i++ {
		oldB.WriteString("\nold " + itoa(i))
		newB.WriteString("\nnew " + itoa(i))
	}
	c := diffSystemPrefix(oldB.String(), newB.String())
	if c == nil || len(c.Areas) != 1 || c.Areas[0].Kind != ContextModified {
		t.Fatalf("want one modified area, got %+v", c)
	}
	if len(c.Areas[0].Lines) > maxAreaLines+1 {
		t.Fatalf("rendered body must stay capped, got %d lines", len(c.Areas[0].Lines))
	}
	if c.Added != n || c.Removed != n {
		t.Fatalf("counts = +%d -%d, want +%d -%d (cap must not truncate them)", c.Added, c.Removed, n, n)
	}
}

// TestDiffSystemPrefixModifiedLineDiff: a paragraph with one changed line yields
// a unified diff showing that line only, with at most lineDiffContext lines of
// surrounding context and an ellipsis for what was elided.
func TestDiffSystemPrefixModifiedLineDiff(t *testing.T) {
	body := func(l5 string) string {
		return "# Rules\nl1\nl2\nl3\nl4\n" + l5 + "\nl6\nl7\nl8\nl9"
	}
	c := diffSystemPrefix(body("OLD"), body("NEW"))
	if c == nil || len(c.Areas) != 1 {
		t.Fatalf("want one area, got %+v", c)
	}
	a := c.Areas[0]
	if a.Kind != ContextModified || a.Label != "Rules" {
		t.Fatalf("want a modified area labelled Rules, got %+v", a)
	}
	want := []string{"…", " l3", " l4", "-OLD", "+NEW", " l6", " l7", "…"}
	if len(a.Lines) != len(want) {
		t.Fatalf("want %v, got %v", want, a.Lines)
	}
	for i, w := range want {
		if a.Lines[i] != w {
			t.Fatalf("line %d: want %q, got %q (all: %v)", i, w, a.Lines[i], a.Lines)
		}
	}
}

// TestDiffSystemPrefixUnpairedBlocks: a wholly new block and a wholly deleted
// one stay added/removed — pairing must not invent a "modified" out of two
// unrelated paragraphs.
func TestDiffSystemPrefixUnpairedBlocks(t *testing.T) {
	old := "# Keep\nsame\n\n# Gone\nalpha beta\ngamma delta"
	nw := "# Keep\nsame\n\n# Fresh\nnothing alike here\nor here either"
	c := diffSystemPrefix(old, nw)
	if c == nil || len(c.Areas) != 2 {
		t.Fatalf("want two unpaired areas, got %+v", c)
	}
	kinds := map[ContextChangeKind]string{}
	for _, a := range c.Areas {
		kinds[a.Kind] = a.Label
	}
	if kinds[ContextRemoved] != "Gone" || kinds[ContextAdded] != "Fresh" {
		t.Fatalf("want -Gone / +Fresh, got %+v", c.Areas)
	}
}

// TestModifiedAreaMultiByteNoPanic: a modified paragraph holding an over-cap
// multi-byte line must truncate by RUNE, not byte (see contextdiff_rune_test).
func TestModifiedAreaMultiByteNoPanic(t *testing.T) {
	long := strings.Repeat("ş", maxAreaLineLen+50)
	old := "# Türkçe\n" + long + "\nson"
	nw := "# Türkçe\n" + long + "\nsonu değişti"
	c := diffSystemPrefix(old, nw) // must not panic
	if c == nil || len(c.Areas) != 1 || c.Areas[0].Kind != ContextModified {
		t.Fatalf("want one modified area, got %+v", c)
	}
	for _, l := range c.Areas[0].Lines {
		// +1 for the diff sign, +1 for the ellipsis added on truncation.
		if n := len([]rune(l)); n > maxAreaLineLen+2 {
			t.Fatalf("line not rune-capped: %d runes", n)
		}
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
