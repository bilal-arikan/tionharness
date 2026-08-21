package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLineDiffCounts(t *testing.T) {
	old := "a\nb\nc\n"
	newText := "a\nB\nc\nd\n"
	added, removed, patch := lineDiff(old, newText)
	// b -> B is one removed + one added; d is one added.
	if added != 2 || removed != 1 {
		t.Fatalf("got added=%d removed=%d, want added=2 removed=1", added, removed)
	}
	if !strings.Contains(patch, "- b") || !strings.Contains(patch, "+ B") || !strings.Contains(patch, "+ d") {
		t.Errorf("patch missing expected lines:\n%s", patch)
	}
}

func TestLineDiffNewFile(t *testing.T) {
	added, removed, _ := lineDiff("", "x\ny\n")
	if added != 2 || removed != 0 {
		t.Errorf("new file: got added=%d removed=%d, want 2/0", added, removed)
	}
}

// TestLineDiffLargeFileSmallEdit pins the maxDiffLines trap: a small edit in a
// file larger than maxDiffLines must report the REAL change, not the whole file
// as added/removed. Regression for SES589 — a 10.5k-line doc with a 35/1 edit
// was showing +10526/−10492 in the UI.
func TestLineDiffLargeFileSmallEdit(t *testing.T) {
	var oldB strings.Builder
	for i := 0; i < 6000; i++ {
		fmt.Fprintf(&oldB, "line %06d\n", i)
	}
	old := oldB.String()
	newText := strings.Replace(old, "line 003000\n", "line 003000\nline 003001\nline 003002\nline 003003\n", 1)

	added, removed, patch := lineDiff(old, newText)
	if added != 3 || removed != 0 {
		t.Fatalf("large-file edit: got added=%d removed=%d, want added=3 removed=0", added, removed)
	}
	if !strings.Contains(patch, "+ line 003001") || !strings.Contains(patch, "+ line 003002") {
		t.Errorf("patch missing inserted lines:\n%s", patch)
	}
}

// TestLineDiffLargeFileReplacement: a single line swapped inside a large file
// counts as one removed + one added, exactly like the small-file case.
func TestLineDiffLargeFileReplacement(t *testing.T) {
	var oldB strings.Builder
	for i := 0; i < 6000; i++ {
		fmt.Fprintf(&oldB, "line %06d\n", i)
	}
	old := oldB.String()
	newText := strings.Replace(old, "line 004200\n", "line 00REPLACED\n", 1)

	added, removed, patch := lineDiff(old, newText)
	if added != 1 || removed != 1 {
		t.Fatalf("large-file replace: got added=%d removed=%d, want 1/1", added, removed)
	}
	if !strings.Contains(patch, "- line 004200") || !strings.Contains(patch, "+ line 00REPLACED") {
		t.Errorf("patch missing replacement lines:\n%s", patch)
	}
}

func TestRecordAndTakeDiff(t *testing.T) {
	ctx, sink := WithDiffSink(context.Background())
	if sink.Take() != nil {
		t.Fatal("fresh sink should be empty")
	}
	recordDiff(ctx, FileDiff{Path: "a.go", Added: 3, Removed: 1})
	got := sink.Take()
	if got == nil || got.Path != "a.go" || got.Added != 3 || got.Removed != 1 {
		t.Fatalf("unexpected diff: %+v", got)
	}
	if sink.Take() != nil {
		t.Error("Take should clear the sink")
	}
}

// TestRecordDiffMultiFile pins the apply_patch case: several files patched in
// one call must all reach the frontend, aggregated into a single FileDiff
// (counts summed, patches concatenated) instead of the earlier files being
// silently overwritten by the last recordDiff call.
func TestRecordDiffMultiFile(t *testing.T) {
	ctx, sink := WithDiffSink(context.Background())
	recordDiff(ctx, FileDiff{Path: "a.go", Added: 3, Removed: 1, Patch: "+ x"})
	recordDiff(ctx, FileDiff{Path: "b.go", Added: 2, Removed: 0, Patch: "+ y", Created: true})
	got := sink.Take()
	if got == nil {
		t.Fatal("expected an aggregated diff")
	}
	if got.Added != 5 || got.Removed != 1 {
		t.Fatalf("got added=%d removed=%d, want added=5 removed=1", got.Added, got.Removed)
	}
	if !got.Created {
		t.Error("Created should be true when any file was created")
	}
	if got.Path != "2 dosya" {
		t.Errorf("got path %q, want \"2 dosya\"", got.Path)
	}
	if !strings.Contains(got.Patch, "@@ a.go @@") || !strings.Contains(got.Patch, "@@ b.go @@") {
		t.Errorf("patch missing per-file separators:\n%s", got.Patch)
	}
	if sink.Take() != nil {
		t.Error("Take should clear the sink")
	}
}
