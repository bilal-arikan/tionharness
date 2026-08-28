package agent

import (
	"strings"
	"testing"
	"time"
)

// driftPrefixes builds a pair of static prefixes with n paragraphs where every
// paragraph was edited: the header (and therefore the paragraph label) is
// relabelled and one body line changes. This is the worst case for pairing —
// pass 1 (exact label) matches nothing, and the three boilerplate body lines put
// EVERY removal above pairSimilarityMin against EVERY addition (3 shared lines
// out of 5), so the similarity pass sees a full n×n candidate list — as long as n
// stays at or below maxSimilarityPairBlocks, past which that pass is skipped.
func driftPrefixes(n int) (frozen, live string) {
	var f, l strings.Builder
	const body = "common line one\ncommon line two\ncommon line three"
	for i := 0; i < n; i++ {
		id := itoa(i)
		f.WriteString("# block " + id + " old\n" + body + "\nold tail " + id + "\n\n")
		l.WriteString("# block " + id + " new\n" + body + "\nnew tail " + id + "\n\n")
	}
	return f.String(), l.String()
}

// TestDiffSystemPrefixDriftOverPairCapShortCircuits covers the cap branch: with
// more changed paragraphs than maxSimilarityPairBlocks the similarity pass is
// skipped wholesale, so this only asserts that the ceiling is in effect — it does
// NOT measure the greedy pairing cost (see the under-cap test for that guard).
func TestDiffSystemPrefixDriftOverPairCapShortCircuits(t *testing.T) {
	frozen, live := driftPrefixes(400)
	start := time.Now()
	c := diffSystemPrefix(frozen, live)
	elapsed := time.Since(start)
	if c == nil {
		t.Fatal("diffSystemPrefix returned nil for a fully rewritten prefix")
	}
	if c.Added != 400 || c.Removed != 400 {
		t.Fatalf("counts = +%d -%d, want +400 -400", c.Added, c.Removed)
	}
	if elapsed > time.Second {
		t.Fatalf("diffSystemPrefix over 400 changed paragraphs took %s, want < 1s", elapsed)
	}
}

// TestDiffSystemPrefixDriftUnderPairCapIsFast is the actual O(n⁴) regression
// guard. At exactly maxSimilarityPairBlocks changed paragraphs the similarity
// pass runs in full: a 200×200 candidate list built and walked through the sorted
// greedy path. The original implementation recomputed line multisets per candidate
// pair and rescanned the candidate list per greedy round; today this costs ~11ms,
// so the 1s bound below fails loudly if that cost comes back.
func TestDiffSystemPrefixDriftUnderPairCapIsFast(t *testing.T) {
	frozen, live := driftPrefixes(maxSimilarityPairBlocks)
	start := time.Now()
	c := diffSystemPrefix(frozen, live)
	elapsed := time.Since(start)
	if c == nil {
		t.Fatal("diffSystemPrefix returned nil for a fully rewritten prefix")
	}
	// Every removal has a same-body counterpart, so pairing must fold them into
	// modified areas rather than emitting separate removed/added ones.
	for _, a := range c.Areas {
		if a.Kind != ContextModified {
			t.Fatalf("area %q kind = %s, want modified", a.Label, a.Kind)
		}
	}
	if elapsed > time.Second {
		t.Fatalf("diffSystemPrefix over %d changed paragraphs took %s, want < 1s",
			maxSimilarityPairBlocks, elapsed)
	}
}

// BenchmarkDiffSystemPrefixDriftUnderPairCap200 measures the real pairing cost:
// 200 blocks is exactly maxSimilarityPairBlocks, so the similarity pass runs.
func BenchmarkDiffSystemPrefixDriftUnderPairCap200(b *testing.B) {
	frozen, live := driftPrefixes(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diffSystemPrefix(frozen, live)
	}
}

// BenchmarkDiffSystemPrefixDriftOverPairCap400 measures the short-circuited path:
// 400 > maxSimilarityPairBlocks, so the similarity pass never runs and this timing
// says nothing about greedy pairing cost.
func BenchmarkDiffSystemPrefixDriftOverPairCap400(b *testing.B) {
	frozen, live := driftPrefixes(400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diffSystemPrefix(frozen, live)
	}
}
