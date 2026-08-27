package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// capSink is a fake ArtifactSink recording what the offload path persisted.
type capSink struct {
	spec CreateArtifactSpec
	n    int
	err  error
}

func (s *capSink) CreateArtifact(_ context.Context, spec CreateArtifactSpec) (ArtifactRef, error) {
	s.n++
	if s.err != nil {
		return ArtifactRef{}, s.err
	}
	s.spec = spec
	return ArtifactRef{ID: "ART42", Title: spec.Title, Kind: spec.Kind}, nil
}

func (s *capSink) UpdateArtifact(_ context.Context, _, _ string) (ArtifactRef, error) {
	return ArtifactRef{}, errors.New("not used")
}

func TestCapToolOutput(t *testing.T) {
	// Under the cap: returned verbatim.
	small := "hello world"
	if got := capToolOutput(small); got != small {
		t.Fatalf("small output altered: %q", got)
	}

	// Over the cap: truncated with a marker, never longer than cap + marker.
	big := strings.Repeat("a", maxToolOutputBytes+5000)
	got := capToolOutput(big)
	if len(got) >= len(big) {
		t.Fatalf("oversized output not truncated: len=%d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("missing truncation marker: %q", got[len(got)-40:])
	}
}

// misalignedBody builds an oversized body whose rune layout guarantees that
// EVERY truncation budget lands in the middle of a multi-byte rune: an ASCII
// prefix of 0..2 bytes followed by 3-byte U+2026 runes.
//
// A uniform 2-byte filler is useless here: both budgets are even, so the cut
// always falls on a rune boundary and the UTF-8 trim loops in truncHead /
// truncTail never execute — the test would pass with those loops deleted.
// The prefix length is picked so that neither the plain cap nor the offload
// head budget is congruent to it modulo 3, which also misaligns the offload
// tail budget (tail = cap - len(head), so tail % 3 == (cap - prefix) % 3).
func misalignedBody(t *testing.T) string {
	t.Helper()
	headBudget := int(float64(maxToolOutputBytes) * offloadHeadBudget)
	for prefix := 0; prefix < 3; prefix++ {
		if (maxToolOutputBytes-prefix)%3 != 0 && (headBudget-prefix)%3 != 0 {
			// 3 bytes per rune * maxToolOutputBytes runes → far above the cap.
			return strings.Repeat("x", prefix) + strings.Repeat("…", maxToolOutputBytes)
		}
	}
	t.Fatalf("no misaligning prefix for cap=%d head=%d", maxToolOutputBytes, headBudget)
	return ""
}

// checkCut asserts that cut is the MAXIMAL valid-UTF-8 slice fitting in budget:
// valid, at most budget bytes, no more than 2 bytes short of it — and strictly
// shorter than budget, which is what proves the trim loop actually ran.
func checkCut(t *testing.T, what, cut string, budget int) {
	t.Helper()
	if !utf8.ValidString(cut) {
		t.Fatalf("%s is not valid UTF-8 (len=%d)", what, len(cut))
	}
	if len(cut) > budget {
		t.Fatalf("%s overflows its budget: len=%d budget=%d", what, len(cut), budget)
	}
	if len(cut) == budget {
		t.Fatalf("%s ends exactly on the budget: the input did not misalign the cut, "+
			"so the UTF-8 trim loop was never exercised (len=%d)", what, len(cut))
	}
	if len(cut) < budget-2 {
		t.Fatalf("%s dropped more than one partial rune: len=%d budget=%d", what, len(cut), budget)
	}
}

func TestCapToolOutputRuneBoundary(t *testing.T) {
	// A multibyte rune straddling the cut point must not produce invalid UTF-8.
	body := misalignedBody(t)
	got := capToolOutput(body)
	marker := strings.Index(got, "\n…[truncated")
	if marker < 0 {
		t.Fatalf("expected truncation marker")
	}
	cut := got[:marker]
	if !strings.HasPrefix(body, cut) {
		t.Fatalf("truncated prefix is not a valid prefix of the input")
	}
	checkCut(t, "truncated head", cut, maxToolOutputBytes)
	if !strings.HasSuffix(cut, "…") {
		t.Fatalf("truncated head does not end on a complete rune")
	}
}

// Without a sink on ctx the offload path must behave exactly like plain capping.
func TestCapToolOutputOffloadNoSink(t *testing.T) {
	ctx := context.Background()

	small := "hello world"
	if got := capToolOutputOffload(ctx, "shell", small); got != small {
		t.Fatalf("small output altered without a sink: %q", got)
	}

	big := strings.Repeat("a", maxToolOutputBytes+5000)
	got := capToolOutputOffload(ctx, "shell", big)
	if want := capToolOutput(big); got != want {
		t.Fatalf("no-sink offload diverged from plain truncation:\n got %q\nwant %q",
			got[len(got)-60:], want[len(want)-60:])
	}
}

// Over the cap with a sink: the FULL output is persisted and the model gets
// head + tail + the artifact handle.
func TestCapToolOutputOffloadWritesArtifact(t *testing.T) {
	head := strings.Repeat("H", 100)
	middle := strings.Repeat("m", maxToolOutputBytes+5000)
	tail := strings.Repeat("T", 100)
	full := head + middle + tail

	sink := &capSink{}
	got := capToolOutputOffload(WithArtifacts(context.Background(), sink), "mcp__x__y", full)

	if sink.n != 1 {
		t.Fatalf("CreateArtifact called %d times, want 1", sink.n)
	}
	if sink.spec.Content != full {
		t.Fatalf("artifact content is not the full output: got %d bytes, want %d",
			len(sink.spec.Content), len(full))
	}
	if sink.spec.Kind != "text" {
		t.Fatalf("artifact kind = %q, want text", sink.spec.Kind)
	}
	if !strings.Contains(sink.spec.Title, "mcp__x__y") {
		t.Fatalf("artifact title does not name the tool: %q", sink.spec.Title)
	}
	if !strings.Contains(got, "artifact ART42") {
		t.Fatalf("returned text carries no artifact handle: %q", got)
	}
	if !strings.HasPrefix(got, head) {
		t.Fatalf("head of the output was not preserved")
	}
	if !strings.HasSuffix(got, tail) {
		t.Fatalf("tail of the output was not preserved")
	}
	if len(got) <= maxToolOutputBytes/2 || len(got) >= len(full) {
		t.Fatalf("offloaded text has implausible length %d (cap %d, full %d)",
			len(got), maxToolOutputBytes, len(full))
	}
}

// The head/tail split must not cut a multi-byte rune in half at either end.
func TestCapToolOutputOffloadRuneBoundary(t *testing.T) {
	full := misalignedBody(t)
	sink := &capSink{}
	got := capToolOutputOffload(WithArtifacts(context.Background(), sink), "shell", full)
	if !utf8.ValidString(got) {
		t.Fatalf("offloaded text is not valid UTF-8")
	}

	// Split the returned text back into the head and tail the offload kept.
	const elideOpen, elideClose = "\n…[", "]…\n"
	start := strings.Index(got, elideOpen)
	end := strings.LastIndex(got, elideClose)
	if start < 0 || end < start {
		t.Fatalf("offloaded text carries no elision marker")
	}
	head, tail := got[:start], got[end+len(elideClose):]

	headBudget := int(float64(maxToolOutputBytes) * offloadHeadBudget)
	if !strings.HasPrefix(full, head) {
		t.Fatalf("kept head is not a prefix of the input")
	}
	checkCut(t, "offload head", head, headBudget)
	if !strings.HasSuffix(head, "…") {
		t.Fatalf("offload head does not end on a complete rune")
	}

	tailBudget := maxToolOutputBytes - len(head)
	if !strings.HasSuffix(full, tail) {
		t.Fatalf("kept tail is not a suffix of the input")
	}
	checkCut(t, "offload tail", tail, tailBudget)
	if !strings.HasPrefix(tail, "…") {
		t.Fatalf("offload tail does not start on a complete rune")
	}
}

// A failing sink must not fail the tool call: it logs and degrades to plain
// truncation, which is the pre-existing behaviour.
func TestCapToolOutputOffloadSinkErrorFallsBack(t *testing.T) {
	big := strings.Repeat("a", maxToolOutputBytes+5000)
	sink := &capSink{err: errors.New("disk full")}
	got := capToolOutputOffload(WithArtifacts(context.Background(), sink), "shell", big)

	if sink.n != 1 {
		t.Fatalf("CreateArtifact called %d times, want 1", sink.n)
	}
	if want := capToolOutput(big); got != want {
		t.Fatalf("failed sink did not fall back to plain truncation: %q", got[len(got)-60:])
	}
}
