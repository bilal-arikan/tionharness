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

func TestCapToolOutputRuneBoundary(t *testing.T) {
	// A multibyte rune straddling the cut point must not produce invalid UTF-8.
	body := strings.Repeat("é", maxToolOutputBytes) // 2 bytes each → way over cap
	got := capToolOutput(body)
	marker := strings.Index(got, "\n…[truncated")
	if marker < 0 {
		t.Fatalf("expected truncation marker")
	}
	if !strings.HasPrefix(body, got[:marker]) {
		t.Fatalf("truncated prefix is not a valid prefix of the input")
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
	full := strings.Repeat("é", maxToolOutputBytes) // 2 bytes each → well over cap
	sink := &capSink{}
	got := capToolOutputOffload(WithArtifacts(context.Background(), sink), "shell", full)
	if !utf8.ValidString(got) {
		t.Fatalf("offloaded text is not valid UTF-8")
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
