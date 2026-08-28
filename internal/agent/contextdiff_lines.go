package agent

import (
	"sort"
	"strings"
)

// Line-level diff for a MODIFIED paragraph.
//
// The paragraph-level LCS in contextdiff.go can only say "this block is gone"
// and "this block is new". When a block was merely edited that reads as a full
// removal plus a full re-addition, and the user has to eyeball two walls of
// identical text to spot the one changed line. This file pairs those two halves
// back together and renders the pair as a unified diff: only the changed lines,
// each with at most a couple of lines of surrounding context, so the actual edit
// is visible at a glance.

// Pairing and rendering knobs. Both are deliberately small: the rendered area
// rides the same caps as any other context area (maxAreaLines / maxAreaLineLen).
const (
	// lineDiffContext is how many unchanged lines are kept on each side of a
	// change; everything further away collapses into a single ellipsis line.
	lineDiffContext = 2
	// pairSimilarityMin is the minimum shared-line ratio for two paragraphs to
	// count as two forms of the same block rather than an unrelated pair.
	pairSimilarityMin = 0.5
	// maxSimilarityPairBlocks caps the similarity pass (pass 2), which is
	// inherently quadratic: it scores every still-unmatched removal against every
	// still-unmatched addition. Above this many blocks on either side the pass is
	// skipped entirely and those paragraphs stay plain added/removed areas — the
	// same output the pairing would have produced for anything it found no
	// counterpart for. That is a fair trade: a drift episode with hundreds of
	// relabelled blocks is a wholesale prefix rewrite, where per-block line diffs
	// carry no signal anyway, and only maxContextAreas (12) areas survive the cap
	// downstream. The exact-label pass 1 has no such cap — it is linear.
	maxSimilarityPairBlocks = 200
)

// paragraphLabel is the self-label of a paragraph: its first non-empty line with
// markdown/quote decoration stripped, rune-truncated.
func paragraphLabel(p string) string {
	label := ""
	for _, l := range strings.Split(p, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			label = t
			break
		}
	}
	return truncateRunes(strings.TrimSpace(strings.TrimLeft(label, "#> \t")), contextLabelMaxRune)
}

// pairParagraphs matches removed paragraphs to added ones that are the same
// block in a new form. Returns pairTo[i] = index into added for removed[i], or
// -1 when that removal has no counterpart. Each added paragraph is used at most
// once.
//
// Two passes, both deterministic: first an exact label match (the common case —
// a block keeps its header and its body changes), then a shared-line similarity
// match for relabelled blocks, taken greedily from the highest score down.
func pairParagraphs(removed, added []string) []int {
	pairTo := make([]int, len(removed))
	for i := range pairTo {
		pairTo[i] = -1
	}
	usedAdd := make([]bool, len(added))

	// Pass 1 — identical labels. Each label keeps its additions in index order, and
	// a per-label cursor walks them, so "first still-unused addition with this
	// label" costs a map lookup instead of a scan over every addition.
	byLabel := make(map[string][]int, len(added))
	for j, p := range added {
		if lbl := paragraphLabel(p); lbl != "" {
			byLabel[lbl] = append(byLabel[lbl], j)
		}
	}
	cursor := make(map[string]int, len(byLabel))
	for i, p := range removed {
		lbl := paragraphLabel(p)
		if lbl == "" {
			continue
		}
		js := byLabel[lbl]
		k := cursor[lbl]
		for k < len(js) && usedAdd[js[k]] {
			k++
		}
		cursor[lbl] = k
		if k < len(js) {
			pairTo[i] = js[k]
			usedAdd[js[k]] = true
			cursor[lbl] = k + 1
		}
	}

	// Pass 2 — shared-line similarity for whatever is still unmatched. Skipped
	// wholesale past the block cap; see maxSimilarityPairBlocks.
	if len(removed) > maxSimilarityPairBlocks || len(added) > maxSimilarityPairBlocks {
		return pairTo
	}
	// Line multisets are computed once per paragraph, not once per candidate pair.
	addCounts := make([]map[string]int, len(added))
	for j, p := range added {
		if !usedAdd[j] {
			addCounts[j] = lineCounts(p)
		}
	}
	type cand struct {
		i, j  int
		score float64
	}
	var cands []cand
	for i, rp := range removed {
		if pairTo[i] >= 0 {
			continue
		}
		rl := lineCounts(rp)
		for j := range added {
			if usedAdd[j] {
				continue
			}
			if s := lineSimilarity(rl, addCounts[j]); s >= pairSimilarityMin {
				cands = append(cands, cand{i: i, j: j, score: s})
			}
		}
	}
	// Sorting once by (score desc, i asc, j asc) makes a single forward pass
	// equivalent to repeatedly picking the best remaining candidate — same result
	// as the old rescan-per-round greedy, without its O(k²).
	sort.Slice(cands, func(a, b int) bool {
		x, y := cands[a], cands[b]
		if x.score != y.score {
			return x.score > y.score
		}
		if x.i != y.i {
			return x.i < y.i
		}
		return x.j < y.j
	})
	for _, c := range cands {
		if pairTo[c.i] < 0 && !usedAdd[c.j] {
			pairTo[c.i] = c.j
			usedAdd[c.j] = true
		}
	}
	return pairTo
}

// lineCounts is the multiset of a paragraph's trimmed, non-empty lines.
func lineCounts(p string) map[string]int {
	m := map[string]int{}
	for _, l := range strings.Split(p, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			m[t]++
		}
	}
	return m
}

// lineSimilarity is |A ∩ B| / max(|A|, |B|) over the two line multisets.
func lineSimilarity(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var total, common, bTotal int
	for _, n := range a {
		total += n
	}
	for l, n := range b {
		bTotal += n
		if m := a[l]; m > 0 {
			common += min(m, n)
		}
	}
	den := total
	if bTotal > den {
		den = bTotal
	}
	return float64(common) / float64(den)
}

// modifiedArea renders a paired (old, new) paragraph as a single unified-diff
// area: unchanged lines carry a leading space, removals "-", additions "+", and
// runs of unchanged lines further than lineDiffContext from any change collapse
// into a lone "…".
//
// It also returns how many lines the edit really added and removed. Those counts
// come from the FULL op list, before context trimming and the maxAreaLines cap,
// so the headline "+N -M" describes the edit rather than the rendered excerpt.
func modifiedArea(oldP, newP string) (area ContextArea, addLines, delLines int) {
	label := paragraphLabel(newP)
	if label == "" {
		label = paragraphLabel(oldP)
	}
	ops := lineOps(strings.Split(oldP, "\n"), strings.Split(newP, "\n"))
	for _, op := range ops {
		switch op.sign {
		case '+':
			addLines++
		case '-':
			delLines++
		}
	}
	return ContextArea{Label: label, Kind: ContextModified, Lines: unifiedLineDiff(ops)}, addLines, delLines
}

// diffOp is one line of the unified rendering before context trimming.
type diffOp struct {
	sign byte // ' ', '-' or '+'
	text string
}

// unifiedLineDiff builds the trimmed, capped unified diff body from one
// paragraph pair's ops (the caller keeps the untrimmed ops for counting).
func unifiedLineDiff(ops []diffOp) []string {
	keep := contextMask(ops)

	out := make([]string, 0, len(ops))
	gap := false
	for i, op := range ops {
		if !keep[i] {
			if !gap {
				out = append(out, "…")
				gap = true
			}
			continue
		}
		gap = false
		out = append(out, string(op.sign)+truncateDiffLine(op.text))
		if len(out) >= maxAreaLines {
			if i < len(ops)-1 {
				out = append(out, "…")
			}
			break
		}
	}
	return out
}

// truncateDiffLine caps one body line by RUNE count (see paragraphArea: a
// byte-length guard panics on multi-byte text).
func truncateDiffLine(l string) string {
	if r := []rune(l); len(r) > maxAreaLineLen {
		return string(r[:maxAreaLineLen]) + "…"
	}
	return l
}

// lineOps is a standard LCS diff over lines, emitted in source order.
func lineOps(oldL, newL []string) []diffOp {
	m, n := len(oldL), len(newL)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			switch {
			case oldL[i] == newL[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, m+n)
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case oldL[i] == newL[j]:
			ops = append(ops, diffOp{' ', oldL[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', oldL[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', newL[j]})
			j++
		}
	}
	for ; i < m; i++ {
		ops = append(ops, diffOp{'-', oldL[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, diffOp{'+', newL[j]})
	}
	return ops
}

// contextMask marks which ops survive trimming: every change, plus up to
// lineDiffContext unchanged lines around each one.
func contextMask(ops []diffOp) []bool {
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.sign == ' ' {
			continue
		}
		lo, hi := i-lineDiffContext, i+lineDiffContext
		if lo < 0 {
			lo = 0
		}
		if hi >= len(ops) {
			hi = len(ops) - 1
		}
		for k := lo; k <= hi; k++ {
			keep[k] = true
		}
	}
	return keep
}
