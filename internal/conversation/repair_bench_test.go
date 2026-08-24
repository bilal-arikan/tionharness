package conversation

import (
	"fmt"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// wellFormedHistory builds n turn-pairs: an assistant tool_use answered by a
// user tool_result — the shape a healthy in-flight history actually has.
func wellFormedHistory(pairs int) []providers.Message {
	msgs := make([]providers.Message, 0, pairs*2)
	for i := 0; i < pairs; i++ {
		id := fmt.Sprintf("call-%d", i)
		msgs = append(msgs, providers.Message{
			Role:      providers.RoleAssistant,
			ToolCalls: []providers.ToolCall{{ID: id, Name: "shell"}},
		})
		msgs = append(msgs, providers.Message{
			Role:        providers.RoleUser,
			ToolResults: []providers.ToolResult{{CallID: id, Content: "ok"}},
		})
	}
	return msgs
}

// BenchmarkRepairSequence measures the per-iteration cost the tool loop pays:
// RepairSequence runs before EVERY provider call, over the whole in-flight
// history. The question is whether that O(history) scan is worth optimising
// next to the LLM round-trip it precedes.
func BenchmarkRepairSequence(b *testing.B) {
	for _, pairs := range []int{50, 500, 5000} {
		msgs := wellFormedHistory(pairs)
		b.Run(fmt.Sprintf("msgs=%d", len(msgs)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, notes := RepairSequence(msgs)
				if notes != nil {
					b.Fatal("well-formed history must not be repaired")
				}
			}
		})
	}
}
