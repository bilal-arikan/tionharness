package textutil

import (
	"strings"
	"unicode"
)

// Lexical similarity over model-written text. Two layers of the app face the
// same problem: an LLM re-words the SAME issue every time it reports it, so
// exact-key dedup never fires (insight findings clustered by title, lesson store
// entries keyed by an LLM-invented signature slug). Both need one cheap,
// deterministic "are these the same topic?" test, so it lives here instead of
// being re-implemented per package.
//
// The tokenizer is unicode-aware on purpose: transcripts (and therefore the
// lessons distilled from them) are frequently Turkish, and an ASCII-only split
// would shatter "çalıştır" into sub-three-character noise that gets dropped.

// Tokenize returns the lowercased, stopword-filtered word set of s. Words are
// runs of letters/digits; anything shorter than three runes is dropped as noise.
func Tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	var cur strings.Builder
	runes := 0
	flush := func() {
		if runes >= 3 {
			w := cur.String()
			if !similarityStopwords[w] {
				out[w] = struct{}{}
			}
		}
		cur.Reset()
		runes = 0
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			runes++
			continue
		}
		flush()
	}
	flush()
	return out
}

// Jaccard is |a∩b| / |a∪b|; 0 when both are empty.
func Jaccard(a, b map[string]struct{}) float64 {
	inter := Intersection(a, b)
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// Overlap is the overlap coefficient |a∩b| / min(|a|,|b|); 0 when either is
// empty. Unlike Jaccard it is not diluted by one side carrying extra words, so
// it recognizes a short label inside a longer one ("bash-not-powershell" vs
// "powershell-bash-chaining-operator"). Pair it with a Jaccard floor when a
// merge decision hangs on it — on its own it treats "everything I say you also
// say" as identity.
func Overlap(a, b map[string]struct{}) float64 {
	min := len(a)
	if len(b) < min {
		min = len(b)
	}
	if min == 0 {
		return 0
	}
	return float64(Intersection(a, b)) / float64(min)
}

// Intersection counts the tokens present in both sets.
func Intersection(a, b map[string]struct{}) int {
	if len(b) < len(a) {
		a, b = b, a
	}
	n := 0
	for w := range a {
		if _, ok := b[w]; ok {
			n++
		}
	}
	return n
}

// similarityStopwords are common words that carry no grouping signal — English
// glue plus a few generic terms ("tool", "agent") that appear in almost every
// finding title and would otherwise inflate similarity.
var similarityStopwords = map[string]bool{
	"the": true, "and": true, "but": true, "for": true, "with": true, "that": true,
	"this": true, "into": true, "from": true, "not": true, "was": true, "were": true,
	"are": true, "its": true, "has": true, "had": true, "when": true, "which": true,
	"instead": true, "than": true, "then": true, "even": true, "though": true,
	"tool": true, "tools": true, "agent": true, "model": true, "call": true,
	"calls": true, "called": true, "using": true, "use": true, "used": true,
}
