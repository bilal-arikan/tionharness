package providers

import "encoding/json"

// withoutBetweenToolsThinking drops signed reasoning from replayed assistant
// turns. Sonnet 5.5's between_tools cannot use block_binding to tolerate prefix
// edits; portable text, tool calls, and unrecognized server blocks are retained.
// The caller's stored history is never mutated.
func withoutBetweenToolsThinking(messages []Message) []Message {
	out := append([]Message(nil), messages...)
	for i, message := range out {
		if message.Role != RoleAssistant || len(message.RawContent) == 0 {
			continue
		}
		var blocks []json.RawMessage
		if err := json.Unmarshal(message.RawContent, &blocks); err != nil {
			continue // Preserve malformed input so the normal transport reports it.
		}
		kept := make([]json.RawMessage, 0, len(blocks))
		for _, block := range blocks {
			var tag struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(block, &tag) == nil && (tag.Type == "thinking" || tag.Type == "redacted_thinking") {
				continue
			}
			kept = append(kept, block)
		}
		if len(kept) == len(blocks) {
			continue
		}
		if len(kept) == 0 {
			out[i].RawContent = nil
			continue
		}
		out[i].RawContent, _ = json.Marshal(kept)
	}
	return out
}
