package decider

import (
	"encoding/json"
	"fmt"
)

// prepareState masks secrets in the state and keeps the request inside the
// model's context window. A string state is trimmed from the middle; a
// structured state that does not fit is refused (the caller must shrink it,
// since cutting JSON arbitrarily would change its meaning).
func prepareState(state any, questions map[string]Question, contextTokens int) (any, error) {
	budget := stateBudgetBytes(questions, contextTokens)
	if s, ok := state.(string); ok {
		return TrimMiddle(Redact(s), budget), nil
	}
	// Round-trip through JSON so struct fields are redacted like map entries.
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode decision state: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("decode decision state: %w", err)
	}
	generic = redactValue(generic)
	if out, _ := json.Marshal(generic); len(out) > budget {
		return nil, fmt.Errorf("decision state is %d bytes, over the model's %d-byte budget", len(out), budget)
	}
	return generic, nil
}

// stateBudgetBytes estimates how many bytes of state fit beside the questions.
// 2.5 bytes per token is conservative for mixed Turkish/English text and JSON.
func stateBudgetBytes(questions map[string]Question, contextTokens int) int {
	if contextTokens <= 0 {
		contextTokens = 32000
	}
	qBytes := 0
	if raw, err := json.Marshal(questions); err == nil {
		qBytes = len(raw)
	}
	budget := contextTokens*5/2 - qBytes - 2048
	return max(budget, 4096)
}
