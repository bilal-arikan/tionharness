package providers

// coalescePlainSameRole merges adjacent PLAIN-TEXT messages that share a role
// into a single message, so the transcript never carries two consecutive
// same-role turns.
//
// Strict providers (Anthropic) reject a transcript whose roles don't strictly
// alternate ("messages: roles must alternate"). Back-to-back same-role turns
// arise when several agents answer in one shared thread: each reply persists as
// its own assistant turn, so the stored history can read user → assistant →
// assistant. Without this normalization that history is unsendable.
//
// Only plain text turns are coalesced. Any turn carrying ToolCalls (assistant
// request) or ToolResults (the answering user turn) is left exactly as-is: those
// already alternate by construction, and merging them could break the
// tool_use ↔ tool_result pairing the providers require. The merged text is
// joined with a blank line so multi-agent author tags ("[Ada]: …", "[Kai]: …")
// stay on separate, readable lines.
func coalescePlainSameRole(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		plain := len(m.ToolCalls) == 0 && len(m.ToolResults) == 0
		if plain && len(out) > 0 {
			last := &out[len(out)-1]
			lastPlain := len(last.ToolCalls) == 0 && len(last.ToolResults) == 0
			if lastPlain && last.Role == m.Role {
				switch {
				case m.Text == "":
					// nothing to add
				case last.Text == "":
					last.Text = m.Text
				default:
					last.Text += "\n\n" + m.Text
				}
				continue
			}
		}
		out = append(out, m)
	}
	return out
}
