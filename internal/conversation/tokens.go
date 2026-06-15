package conversation

import "github.com/bilal/swarmgo/internal/db"

// charsPerToken is a rough heuristic: ~4 characters per token for mixed
// English/Turkish text. This avoids a tokenizer dependency; it is used only for
// budgeting and the UI meter, where an estimate is sufficient.
const charsPerToken = 4

// msgOverhead approximates the per-message role/framing token cost.
const msgOverhead = 4

// estimateText approximates the token count of a string.
func estimateText(s string) int {
	return len([]rune(s))/charsPerToken + 1
}

// EstimateTokens approximates the tokens of a summary plus a set of messages.
func EstimateTokens(summary string, msgs []db.Message) int {
	total := estimateText(summary)
	for _, m := range msgs {
		total += estimateText(m.Text) + msgOverhead
	}
	return total
}
