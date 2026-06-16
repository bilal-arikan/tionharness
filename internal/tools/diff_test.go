package tools

import (
	"context"
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
