package tools

import (
	"context"
	"strings"
)

// FileDiff is a structured summary of a single file mutation produced by a
// built-in fs tool (write_file / edit_file). The chat UI renders it as a diff
// card (path + added/removed counts + an optional unified patch) instead of a
// generic tool row.
type FileDiff struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	// Patch is a compact unified-style diff (may be empty for very large files).
	Patch string `json:"patch,omitempty"`
	// Created marks a brand-new file (old content was empty/absent).
	Created bool `json:"created,omitempty"`
}

// diffSink collects the diff produced by the current tool call. One sink is
// attached per call, so it holds at most one diff.
type diffSink struct{ last *FileDiff }

// Take returns the recorded diff (if any) and clears it.
func (s *diffSink) Take() *FileDiff {
	if s == nil {
		return nil
	}
	d := s.last
	s.last = nil
	return d
}

type diffKey struct{}

// WithDiffSink attaches a fresh diff sink to ctx and returns both. File-mutating
// built-in tools call recordDiff to populate it; the caller reads it via Take
// after the tool returns. When no sink is attached (e.g. autonomous runs that
// don't render a trace) recordDiff is a no-op.
func WithDiffSink(ctx context.Context) (context.Context, *diffSink) {
	s := &diffSink{}
	return context.WithValue(ctx, diffKey{}, s), s
}

// recordDiff stores a diff on the sink attached to ctx, if present.
func recordDiff(ctx context.Context, d FileDiff) {
	if s, ok := ctx.Value(diffKey{}).(*diffSink); ok {
		s.last = &d
	}
}

// maxDiffLines caps the LCS computation: above this, computing a full diff is
// too expensive, so we report coarse counts (whole old removed, whole new added)
// without a patch.
const maxDiffLines = 4000

// maxPatchLines bounds the emitted unified patch so a huge change doesn't bloat
// the persisted trace. Beyond it the counts are kept but the patch is dropped.
const maxPatchLines = 400

// lineDiff computes a line-level diff between oldText and newText, returning the
// number of added and removed lines plus a compact unified-style patch. It uses
// a classic LCS table; for inputs larger than maxDiffLines it falls back to
// coarse counts without a patch.
func lineDiff(oldText, newText string) (added, removed int, patch string) {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return len(newLines), len(oldLines), ""
	}

	// LCS length table over lines.
	n, m := len(oldLines), len(newLines)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	// Backtrack to emit the +/-/context lines in order.
	var b strings.Builder
	i, j := 0, 0
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			b.WriteString("  " + oldLines[i] + "\n")
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			b.WriteString("- " + oldLines[i] + "\n")
			removed++
			i++
		} else {
			b.WriteString("+ " + newLines[j] + "\n")
			added++
			j++
		}
	}
	for ; i < n; i++ {
		b.WriteString("- " + oldLines[i] + "\n")
		removed++
	}
	for ; j < m; j++ {
		b.WriteString("+ " + newLines[j] + "\n")
		added++
	}

	patch = b.String()
	if added+removed > maxPatchLines {
		patch = "" // too large to persist; keep the counts only
	}
	return added, removed, patch
}

// splitLines splits text into lines without a trailing empty element for a final
// newline (so "a\n" is one line, not two).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
