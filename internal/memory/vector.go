// Package memory gives agents a recallable long-term store: documents the user
// adds, journal entries logged from activity, and reflections the agent writes
// about itself. Recall uses a pure-Go lexical similarity (token-frequency
// cosine) — no external embedding API, no CGO — so it works fully offline. The
// serialized term vector is cached in each row's `embedding` BLOB; a real
// semantic embedder can be swapped in later behind the same Store interface.
package memory

import (
	"encoding/json"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// minTokenLen drops 1-character tokens, which carry little signal.
const minTokenLen = 2

// stopwords are high-frequency Turkish/English words excluded from vectors so
// similarity reflects meaningful terms rather than glue words.
var stopwords = map[string]bool{
	// English
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "is": true, "are": true, "was": true, "be": true,
	"for": true, "on": true, "with": true, "as": true, "at": true, "it": true,
	"this": true, "that": true, "i": true, "you": true, "he": true, "she": true,
	// Turkish
	"ve": true, "ile": true, "bir": true, "bu": true, "şu": true, "o": true,
	"da": true, "de": true, "ki": true, "için": true, "çok": true, "ama": true,
	"gibi": true, "daha": true, "ben": true, "sen": true, "biz": true, "siz": true,
}

// tokenize lowercases text and splits on non-alphanumeric runes (Unicode-aware,
// so Turkish characters survive), dropping stopwords and tiny tokens.
func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if utf8.RuneCountInString(f) < minTokenLen || stopwords[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// vector is a sparse term-frequency map.
type vector map[string]float64

// buildVector returns the term-frequency vector of a piece of text.
func buildVector(text string) vector {
	v := make(vector)
	for _, tok := range tokenize(text) {
		v[tok]++
	}
	return v
}

// cosine returns the cosine similarity of two term vectors in [0,1].
func cosine(a, b vector) float64 {
	return cosineNorm(a, b, norm(a))
}

// cosineNorm is cosine with the first vector's precomputed norm. Recall passes
// the query norm computed once, so it isn't recomputed for every candidate.
func cosineNorm(a, b vector, anorm float64) float64 {
	if len(a) == 0 || len(b) == 0 || anorm == 0 {
		return 0
	}
	// Iterate the smaller map for the dot product.
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	var dot float64
	for term, wa := range small {
		if wb, ok := large[term]; ok {
			dot += wa * wb
		}
	}
	if dot == 0 {
		return 0
	}
	return dot / (anorm * norm(b))
}

func norm(v vector) float64 {
	var sum float64
	for _, w := range v {
		sum += w * w
	}
	return math.Sqrt(sum)
}

// marshalVector serializes a term vector for the embedding BLOB column.
func marshalVector(v vector) []byte {
	data, _ := json.Marshal(v)
	return data
}

// unmarshalVector restores a term vector from a BLOB; nil/invalid yields nil.
func unmarshalVector(data []byte) vector {
	if len(data) == 0 {
		return nil
	}
	var v vector
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	return v
}
