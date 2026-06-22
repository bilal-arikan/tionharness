package conversation

import (
	"github.com/bilal-arikan/swarmgo/internal/db"
)

// charsPerToken is a rough heuristic: ~4 characters per token for mixed
// English/Turkish prose. This avoids a tokenizer dependency; it is used only for
// budgeting and the UI meter, where an estimate is sufficient.
const charsPerToken = 4

// Dense content (base64, hex, data URIs, minified JSON/code) packs far more
// tokens per character than prose — roughly 1.5 chars/token. Estimating it at
// charsPerToken undercounts by ~60%, which let big encoded tool outputs silently
// overflow the real context window ("session poisoning", CG-9). When a string is
// long and almost whitespace-free we switch to the denser ratio: tokens ≈
// runes*2/3 ≈ runes/1.5 (integer-friendly).
const (
	denseMinRunes      = 256 // short strings aren't worth the denser estimate
	denseWhitespacePct = 3   // < 3% whitespace → treat as packed/encoded
)

// msgOverhead approximates the per-message role/framing token cost.
const msgOverhead = 4

// estimateText approximates the token count of a string, density-aware: prose is
// counted at ~4 chars/token, packed/encoded blobs at ~1.5. Single pass over the
// runes (no []rune allocation).
func estimateText(s string) int {
	runes, spaces := 0, 0
	for _, r := range s {
		runes++
		switch r {
		case ' ', '\n', '\t', '\r':
			spaces++
		}
	}
	if runes == 0 {
		return 1
	}
	if runes >= denseMinRunes && spaces*100/runes < denseWhitespacePct {
		return runes*2/3 + 1
	}
	return runes/charsPerToken + 1
}

// EstimateText is the exported approximation of a single string's token count.
// Used by callers that need a per-field breakdown (e.g. the session info panel)
// rather than the aggregate EstimateTokens.
func EstimateText(s string) int {
	return estimateText(s)
}

// EstimateTokens approximates the tokens of a summary plus a set of messages.
func EstimateTokens(summary string, msgs []db.Message) int {
	total := estimateText(summary)
	for _, m := range msgs {
		total += estimateText(m.Text) + msgOverhead
	}
	return total
}
